package backup

// Backup log files via Baidu Object Storage
// Configs:
//     mode: "bos"
//     remote: bos endpoint
// 	   bucket: bos bucket name
// 	   access_key: bos accessKey
//     access_secret: bos accessSecret

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/Laisky/go-utils"

	"github.com/baidubce/bce-sdk-go/bce"
	"github.com/baidubce/bce-sdk-go/services/bos"
	"github.com/pkg/errors"
)

var (
	uploadChunkSize  int64 = 1000 * 1024 * 1024
	uploadConcurrent int64 = 2
)

type BosArgs struct {
	Remote       string // "https://gz.bcebos.com"
	Bucket       string
	AccessKey    string
	AccessSecret string
}

type bosUploader struct {
	*baseUploader
	args *BosArgs
	cli  *bos.Client
}

func (u *bosUploader) New(st *backupSetting) error {
	u.baseUploader = createBaseUploader(st)
	u.args = &BosArgs{
		Remote:       st.Args["remote"].(string),
		AccessKey:    st.Args["access_key"].(string),
		AccessSecret: st.Args["access_secret"].(string),
		Bucket:       st.Args["bucket"].(string),
	}

	// connect
	var err error
	u.cli, err = Connect2bos(u.args.Remote, u.args.AccessKey, u.args.AccessSecret)
	if err != nil {
		return errors.Wrap(err, "try to connect bos error")
	}

	return nil
}

func (u *bosUploader) isFileExists(objName string) bool {
	_, err := u.cli.GetObjectMeta(u.args.Bucket, objName)
	if realErr, ok := err.(*bce.BceServiceError); ok {
		if realErr.StatusCode == 404 {
			return false
		}
	}
	return true
}

func (u *bosUploader) Upload(fpath string) {
	utils.Logger.Debugf("uploading file %v ...", fpath)
	defer u.Done()

	if utils.Settings.GetBool("dry") {
		utils.Logger.Debugf("upload %v", fpath)
		return
	}

	var (
		objName string
		fsize   int64
		err     error
		r       = ""
	)

	if fsize, err = u.CheckIsFileReady(fpath); err != nil {
		utils.Logger.Errorf("try to get file info error: %+v", err)
		u.AddFaiFile(fpath)
		return
	}

	objName = u.getObjFname(fpath)
	if u.isFileExists(objName) {
		utils.Logger.Errorf("file %v already exists", objName)
		u.AddFaiFile(fpath)
		return
	}

	if fsize < 1024*1024*1024 {
		r, err = u.cli.PutObjectFromFile(u.args.Bucket, objName, fpath, nil) // upload single file
	} else { // file size must greater than 5 MB
		err = u.cli.UploadSuperFile(u.args.Bucket, objName, fpath, "") // upload by multipart
	}
	if err != nil {
		utils.Logger.Errorf("upload file got error: %+v", err)
		u.AddFaiFile(fpath)
		return
	}

	u.AddSucFile(fpath)
	utils.Logger.Infof("success uploaded file %v: %v", fpath, r)
}

func (u *bosUploader) Clean() {
	u.CleanFiles()
	go func() {
		runtime.GC() // bos taken too much memory
		debug.FreeOSMemory()
	}()
}

func (u *bosUploader) getObjFname(fpath string) string {
	_, fname := filepath.Split(fpath)
	return fmt.Sprintf("%v/%v", u.GetName(), fname)
}

func Connect2bos(remote, accessKey, accessSecret string) (c *bos.Client, err error) {
	utils.Logger.Debugf("connect to bos for remote %v", remote)

	c, err = bos.NewClient(accessKey, accessSecret, remote)
	if err != nil {
		return nil, errors.Wrapf(err, "try to connect to bos %v error", remote)
	}
	c.MultipartSize = uploadChunkSize
	c.MaxParallel = uploadConcurrent

	return c, nil
}

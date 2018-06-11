// Package backup backup logs from ES to file system
package backup

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/pkg/errors"
	"github.com/Laisky/go-ramjet/tasks/store"
)

type backupSetting struct {
	Name      string
	Path      string
	Regex     string
	IsReserve bool
	Mode      string
	Args      map[string]interface{}
}

// uploader do the uploading
type uploader interface {
	New(*backupSetting) error
	Upload(string)
	Add(int)
	Wait()
	Clean()
}

type baseUploader struct {
	wg             *sync.WaitGroup
	ST             *backupSetting
	successedFiles []string
	failedFiles    []string
}

func createBaseUploader(st *backupSetting) *baseUploader {
	return &baseUploader{
		wg: &sync.WaitGroup{},
		ST: st,
	}
}

func (u *baseUploader) GetName() string {
	return u.ST.Name
}

func (u *baseUploader) Wait() {
	u.wg.Wait()
}

func (u *baseUploader) Add(n int) {
	u.wg.Add(n)
}

func (u *baseUploader) Done() {
	u.wg.Done()
}

// AddSucFile save successed file
func (u *baseUploader) AddSucFile(fpath string) {
	u.successedFiles = append(u.successedFiles, fpath)
}

// AddFaiFile save failed file
func (u *baseUploader) AddFaiFile(fpath string) {
	u.failedFiles = append(u.failedFiles, fpath)
}

// CleanFiles remove failed files if IsReserve=false
func (u *baseUploader) CleanFiles() {

	if u.ST.IsReserve {
		return
	}
	for _, fpath := range u.successedFiles {
		if err := os.Remove(fpath); err != nil {
			utils.Logger.Errorf("remove file got error: %+v", err)
		}
		utils.Logger.Infof("remove file: %v", fpath)
	}
}

// CheckIsFileReady wait the file until it is ready for uploading
func (u *baseUploader) CheckIsFileReady(fpath string) (fsize int64, err error) {
	for {
		fi, err := os.Stat(fpath)
		if err != nil {
			return 0, errors.Wrap(err, "try to get file info error")
		}
		if fi.Size() != fsize {
			fsize = fi.Size()
			time.Sleep(time.Second * 1)
			continue
		}

		return fsize, nil
	}
}

var (
	interval      time.Duration
	uploadTimeout = 1 * time.Second
	backupLock    = &sync.Mutex{}
)

func LoadSettings() (configs []*backupSetting) {
	interval = utils.Settings.GetDuration("tasks.backups.interval") * time.Second
	for name, ci := range utils.Settings.Get("tasks.backups.configs").(map[string]interface{}) {
		c := ci.(map[string]interface{})
		configs = append(configs, &backupSetting{
			Name:      name,
			Path:      c["path"].(string),
			Regex:     c["regex"].(string),
			IsReserve: c["reserve"].(bool),
			Mode:      c["mode"].(string),
			Args:      c,
		})
	}
	return
}

// IsFileReadyToUpload Check is file ready to upload
func IsFileReadyToUpload(regex, fname string, now time.Time) (ok bool, err error) {
	r := regexp.MustCompile(regex)
	subS := r.FindStringSubmatch(fname)

	if len(subS) < 2 {
		return false, nil
	}

	lastFinishedBackupFileBeforeHours := 35.0 // 24 + 11,last flush occured at 10:00 CST
	t, err := time.Parse("20060102-0700", subS[1]+"+0800")
	if err != nil {
		return false, errors.Wrap(err, "parse file time error")
	}

	if now.Sub(t).Hours() <= lastFinishedBackupFileBeforeHours {
		return false, nil
	}

	return true, nil
}

// ScanFiles return absolute file pathes that match regex
func ScanFiles(dir, regex string) (files []string) {
	utils.Logger.Debugf("ScanFiles for dir %v, regex %v", dir, regex)

	if err := filepath.Walk(dir, func(fname string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrapf(err, "scan files got error for dir %v", dir)
		}
		if info.IsDir() {
			return nil
		}

		if ok, err := IsFileReadyToUpload(regex, info.Name(), time.Now()); err != nil {
			return errors.Wrapf(err, "Check file name error")
		} else if !ok {
			return nil
		}

		absPath, err := filepath.Abs(filepath.Join(dir, info.Name()))
		if err != nil {
			return errors.Wrap(err, "get absolute file path error")
		}
		files = append(files, absPath)
		return nil
	}); err != nil {
		utils.Logger.Errorf("scan files got error: %v", err)
	}

	return
}

func runTask() {
	defer backupLock.Unlock()
	backupLock.Lock()

	utils.Logger.Info("start backup elasticsearch logs...")
	LoadSettings() // reload

	var (
		fpath   string
		loader  uploader
		loaders = []uploader{}
		err     error
	)

	for _, st := range LoadSettings() { //reload settings
		switch st.Mode {
		case "rsync":
			loader = &rsyncUploader{}
		case "bos":
			loader = &bosUploader{}
		default:
			utils.Logger.Errorf("got unknown upload mode: %v", st.Mode)
			continue
		}
		err = loader.New(st)
		if err != nil {
			utils.Logger.Errorf("construct uploader error: %+v", err)
			continue
		}

		for _, fpath = range ScanFiles(st.Path, st.Regex) {
			loader.Add(1)
			go loader.Upload(fpath)
		}
		loaders = append(loaders, loader)
	}

	for _, loader = range loaders {
		loader.Wait()
		loader.Clean()
	}
}

func bindTask() {
	utils.Logger.Info("bind backup es logs task...")
	if utils.Settings.GetBool("debug") {
		utils.Settings.Set("tasks.backups.interval", 1)
	}

	LoadSettings()
	go store.TickerAfterRun(interval, runTask)
}

func init() {
	store.Store("backup", bindTask)
}

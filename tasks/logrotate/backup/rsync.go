package backup

// Backup log files via rsync
// Configs:
//     mode: "rsync"
//     remote: rsync server host and path

import (
	"context"
	"os/exec"
	"regexp"

	"github.com/Laisky/go-utils"
	"github.com/pkg/errors"
)

type rsyncArgs struct {
	Remote string // 172.16.0.123::ivilog_bak
}

type rsyncUploader struct {
	*baseUploader
	args *rsyncArgs
}

func (u *rsyncUploader) New(st *backupSetting) error {
	u.baseUploader = createBaseUploader(st)
	u.args = &rsyncArgs{
		Remote: st.Args["remote"].(string),
	}
	return nil
}

func (u *rsyncUploader) Upload(fpath string) {
	utils.Logger.Debugf("uploading file %v ...", fpath)
	defer u.Done()

	var (
		err   error
		fsize int64
	)

	if fsize, err = u.CheckIsFileReady(fpath); err != nil {
		utils.Logger.Errorf("try to get file info error: %+v", err)
		u.AddFaiFile(fpath)
		return
	}

	utils.Logger.Debugf("try to upload file via rsync for %vB", fsize)
	out, err := RunSysCMD(GenRsyncCMD(fpath, u.args.Remote))
	if err != nil {
		utils.Logger.Errorf("run upload cmd error: %+v", err)
		u.AddFaiFile(fpath)
		return
	}

	if matched, err := regexp.MatchString("", out); !matched || err != nil {
		utils.Logger.Errorf("upload got stderr: %+v", err)
		u.AddFaiFile(fpath)
		return
	}

	u.AddSucFile(fpath)
	utils.Logger.Infof("success uploaded file: %v", fpath)
}

func (u *rsyncUploader) Clean() {
	u.CleanFiles()
}

func GenRsyncCMD(fpath, remote string) (cmd []string) {
	return []string{"rsync", "-tvhz", fpath, remote}
}

func RunSysCMD(cmd []string) (output string, err error) {
	if utils.Settings.GetBool("debug") {
		utils.Logger.Debugf("run cmd: %v", cmd)
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), uploadTimeout)
	defer cancel()
	term := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	out, err := term.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", errors.Wrap(ctx.Err(), "upload timeout")
	}
	if err != nil {
		return "", errors.Wrapf(err, "run cmd `%v` got error", cmd)
	}

	utils.Logger.Debugf("success run cmd %v: got %v", cmd, string(out[:]))
	return string(out[:]), nil
}

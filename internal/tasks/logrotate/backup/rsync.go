package backup

// Backup log files via rsync
// Configs:
//     mode: "rsync"
//     remote: rsync server host and path

import (
	"context"
	"os/exec"
	"regexp"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
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
	log.Logger.Debug("uploading file...", zap.String("fpath", fpath))
	defer u.Done()

	var (
		err   error
		fsize int64
	)

	if fsize, err = u.CheckIsFileReady(fpath); err != nil {
		log.Logger.Error("try to get file info error", zap.Error(err))
		u.AddFaiFile(fpath)
		return
	}

	log.Logger.Debug("try to upload file via rsync", zap.Int64("fsize", fsize))
	out, err := RunSysCMD(GenRsyncCMD(fpath, u.args.Remote))
	if err != nil {
		log.Logger.Error("run upload cmd error", zap.Error(err))
		u.AddFaiFile(fpath)
		return
	}

	if matched, err := regexp.MatchString("", out); !matched || err != nil {
		log.Logger.Error("upload got stderr", zap.Error(err))
		u.AddFaiFile(fpath)
		return
	}

	u.AddSucFile(fpath)
	log.Logger.Info("success uploaded file", zap.String("fpath", fpath))
}

func (u *rsyncUploader) Clean() {
	u.CleanFiles()
}

func GenRsyncCMD(fpath, remote string) (cmd []string) {
	return []string{"rsync", "-tvhz", fpath, remote}
}

func RunSysCMD(cmd []string) (output string, err error) {
	if utils.Settings.GetBool("debug") {
		log.Logger.Debug("run cmd", zap.Strings("cmd", cmd))
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

	log.Logger.Debug("success run", zap.Strings("cmd", cmd), zap.String("out", string(out[:])))
	return string(out[:]), nil
}

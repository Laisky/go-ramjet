// Package backup backup logs from ES to file system
package backup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/go-ramjet/tasks/store"
)

var (
	interval      time.Duration
	isDone        = uint32(0)
	uploadTimeout = 1 * time.Second
)

type backupSetting struct {
	Path      string
	Regex     string
	IsReserve bool
	Mode      string
	Remote    string
}

func loadSettings() (configs []*backupSetting) {
	interval = viper.GetDuration("tasks.backups.interval")
	for _, ci := range viper.Get("tasks.backups.configs").(map[string]interface{}) {
		c := ci.(map[string]interface{})
		configs = append(configs, &backupSetting{
			Path:      c["path"].(string),
			Regex:     c["regex"].(string),
			IsReserve: c["reserve"].(bool),
			Mode:      c["mode"].(string),
			Remote:    c["remote"].(string),
		})
	}
	return
}

func genRsyncCMD(fpath, remote string) (cmd []string) {
	return []string{"rsync", "-tvhz", fpath, remote}
}

// ScanFiles return absolute file pathes that match regex
func scanFiles(dir, regex string) (files []string) {
	defer log.Flush()
	log.Debugf("scanFiles for dir %v, regex %v", dir, regex)

	if err := filepath.Walk(dir, func(fname string, info os.FileInfo, err error) error {
		if err != nil {
			panic(errors.Wrapf(err, "prevent panic by handling failure accessing a path %q", dir))
		}
		if info.IsDir() {
			return nil
		}

		matched, err := regexp.MatchString(regex, info.Name())
		if err != nil {
			panic(errors.Wrap(err, "regex compare with the file error"))
		}
		if !matched {
			return nil
		}

		absPath, err := filepath.Abs(filepath.Join(dir, info.Name()))
		if err != nil {
			panic(errors.Wrap(err, "get absolute file path error"))
		}
		files = append(files, absPath)
		return nil
	}); err != nil {
		panic(errors.Wrap(err, "scan files"))
	}

	return files
}

func runSysCMD(cmd []string) (output string, err error) {
	defer log.Flush()
	if viper.GetBool("debug") {
		log.Debugf("run cmd: %v", cmd)
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

	log.Debugf("success run cmd %v: got %v", cmd, string(out[:]))
	return string(out[:]), nil
}

func uploadFileByRsync(wg *sync.WaitGroup, fpath, remote string, isReserve bool) {
	defer log.Flush()
	defer wg.Done()
	log.Debugf("uploading file %v ...", fpath)

	out, err := runSysCMD(genRsyncCMD(fpath, remote))
	if err != nil {
		log.Errorf("run upload cmd error: %+v", err)
		return
	}

	if matched, err := regexp.MatchString("", out); !matched || err != nil {
		log.Errorf("upload got stderr: %+v", err)
		return
	}

	log.Infof("success uploaded file: %v", fpath)
	if !isReserve {
		if err := os.Remove(fpath); err != nil {
			log.Errorf("remove file got error: %+v", err)
		}
		log.Infof("remove file: %v", fpath)
	}
}

func runTask() {
	defer log.Flush()

	if ok := atomic.CompareAndSwapUint32(&isDone, 0, 1); !ok {
		log.Info("another task is still running, exit...")
		return
	}
	defer func() {
		if ok := atomic.CompareAndSwapUint32(&isDone, 1, 0); !ok {
			panic("set task done error")
		}
	}()

	log.Debug("runTask")
	loadSettings() // reload

	var (
		fpath string
		wg    = &sync.WaitGroup{}
	)

	for _, st := range loadSettings() {
		if st.Mode != "rsync" {
			log.Warn("only support `rsync` mode now")
			continue
		}
		for _, fpath = range scanFiles(st.Path, st.Regex) {
			wg.Add(1)
			go uploadFileByRsync(wg, fpath, st.Remote, st.IsReserve)
		}
	}

	wg.Wait()
}

func bindTask() {
	defer log.Flush()
	log.Info("bind backup es logs task...")
	if viper.GetBool("debug") {
		viper.Set("tasks.backups.interval", 1)
	}

	go store.Ticker(interval, runTask)
}

func init() {
	store.Store(bindTask)
}

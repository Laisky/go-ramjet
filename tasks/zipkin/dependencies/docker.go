package dependencies

import (
	"bytes"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var DEFAULT_ENV = []string{
	"JAVA_OPTS=-XX:+UseG1GC -Xss256k -XX:G1ConcRefinementThreads=2 -XX:CICompilerCount=4 -XX:ParallelGCThreads=4 -XX:MaxMetaspaceSize=128m -XX:CompressedClassSpaceSize=32m",
	"TZ=Asia/Shanghai",
	"reserved_megabytes=256",
	"STORAGE_TYPE=elasticsearch",
}

func generateContainerEnv(host, index, username, passwd string) []string {
	return []string{
		"ES_HOSTS=" + host,
		"ES_INDEX=" + index,
		"ES_USERNAME=" + username,
		"ES_PASSWORD=" + passwd,
	}
}

// runDockerContainer running container by image name, and return stdout & stderr
func runDockerContainer(endpoint, image string, env []string) (stdout, stderr []byte, err error) {
	utils.Logger.Info("runDockerContainer", zap.String("image", image), zap.String("endpoint", endpoint))
	env = append(DEFAULT_ENV, env...)
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to create docker cli got error")
	}

	hostcfg := &docker.HostConfig{
		NetworkMode: "host",
		AutoRemove:  false,
	}

	container, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: image,
			Env:   env,
		},
		HostConfig: hostcfg,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to create docker container got error")
	}

	startTs := time.Now()
	err = client.StartContainer(container.ID, hostcfg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to starting container got error")
	}
	utils.Logger.Info("successed running container",
		zap.Duration("cost", time.Now().Sub(startTs)),
		zap.String("image", image))

	_, err = client.WaitContainer(container.ID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to wait container got error")
	}

	var stdoutb, stderrb bytes.Buffer
	err = client.Logs(docker.LogsOptions{
		Container:    container.ID,
		Stdout:       true,
		Stderr:       true,
		OutputStream: &stdoutb,
		ErrorStream:  &stderrb,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to load container log got error")
	}

	err = client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            container.ID,
		RemoveVolumes: true,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to remove container got error")
	}

	return stdoutb.Bytes(), stderrb.Bytes(), nil
}

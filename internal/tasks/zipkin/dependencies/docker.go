package dependencies

import (
	"bytes"
	"strings"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	docker "github.com/fsouza/go-dockerclient"
)

var DefaultEnvs = []string{
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

func SplitImage2RepoAndTag(image string) (repo, tag string) {
	images := strings.Split(image, "/")
	name := ""
	if len(images) == 1 {
		name = image
	} else {
		repo = images[0]
		name = images[1]
	}

	if !strings.Contains(name, ":") {
		tag = "latest"
	} else {
		tag = strings.Split(name, ":")[1]
		name = strings.Split(name, ":")[0]
	}

	if repo != "" {
		repo += "/" + name
	} else {
		repo = name
	}

	return
}

// runDockerContainer running container by image name, and return stdout & stderr
func runDockerContainer(endpoint, image string, env []string) (stdout, stderr []byte, err error) {
	log.Logger.Info("runDockerContainer", zap.String("image", image), zap.String("endpoint", endpoint))
	env = append(DefaultEnvs, env...)
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to create docker cli got error")
	}

	hostcfg := &docker.HostConfig{
		NetworkMode: "host",
		AutoRemove:  false,
	}

	log.Logger.Debug("pulling image", zap.String("image", image))
	repo, tag := SplitImage2RepoAndTag(image)
	err = client.PullImage(docker.PullImageOptions{
		Repository: repo,
		Tag:        tag,
	}, docker.AuthConfiguration{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to pull image got error")
	}

	log.Logger.Debug("creating container", zap.String("image", image))
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

	log.Logger.Debug("running container", zap.String("image", image))
	startTs := time.Now()
	err = client.StartContainer(container.ID, hostcfg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try to starting container got error")
	}
	log.Logger.Info("successed running container",
		zap.Duration("cost", time.Since(startTs)),
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

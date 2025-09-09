package dependencies_test

import (
	"bytes"
	"os"
	"testing"

	docker "github.com/fsouza/go-dockerclient"

	"github.com/Laisky/go-ramjet/internal/tasks/zipkin/dependencies"
)

func TestDocker(t *testing.T) {
	if os.Getenv("RUN_DOCKER_IT") == "" {
		t.Skip("integration test disabled: set RUN_DOCKER_IT to run")
	}
	endpoint := "unix:///var/run/docker.sock"
	client, err := docker.NewClient(endpoint)
	if err != nil {
		t.Fatalf("got err: %+v", err)
	}

	hostcfg := &docker.HostConfig{
		NetworkMode: "host",
		AutoRemove:  false,
	}

	c, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: "hello-world",
			// Cmd:          []string{"/bin/sleep", "10"},
			AttachStderr: true,
			AttachStdout: true,
			Env:          []string{"PATH=/bin"},
		},
		HostConfig: hostcfg,
	})
	if err != nil {
		t.Fatalf("got err: %+v", err)
	}
	t.Logf("container stat: %v", c.State.String())

	err = client.StartContainer(c.ID, hostcfg)
	if err != nil {
		t.Fatalf("got err: %+v", err)
	}
	t.Logf("container stat: %v", c.State.String())

	_, err = client.WaitContainer(c.ID)
	if err != nil {
		t.Fatalf("got err: %+v", err)
	}

	var stdout, stderr bytes.Buffer
	err = client.Logs(docker.LogsOptions{
		Container:    c.ID,
		Stdout:       true,
		Stderr:       true,
		OutputStream: &stdout,
		ErrorStream:  &stderr,
	})
	if err != nil {
		t.Fatalf("got err: %+v", err)
	}

	t.Logf("got logs: %v", stdout.String())
	t.Logf("got errs: %v", stderr.String())
	t.Error("done")
}

func TestSplitImage2RepoAndTag(t *testing.T) {
	image := "registry:5000/zipkin-dependencies:2.0.4"
	repo, tag := dependencies.SplitImage2RepoAndTag(image)
	if repo != "registry:5000/zipkin-dependencies" {
		t.Fatalf("expect %v, got %v\n", "registry:5000/zipkin-dependencies", repo)
	}
	if tag != "2.0.4" {
		t.Fatalf("expect %v, got %v\n", "2.0.4", tag)
	}

	image = "helloworld"
	repo, tag = dependencies.SplitImage2RepoAndTag(image)
	if repo != "helloworld" {
		t.Fatalf("expect %v, got %v\n", "helloworld", repo)
	}
	if tag != "latest" {
		t.Fatalf("expect %v, got %v\n", "latest", tag)
	}

	image = "helloworld:1.0"
	repo, tag = dependencies.SplitImage2RepoAndTag(image)
	if repo != "helloworld" {
		t.Fatalf("expect %v, got %v\n", "helloworld", repo)
	}
	if tag != "1.0" {
		t.Fatalf("expect %v, got %v\n", "1.0", tag)
	}
}

package dependencies_test

import (
	"bytes"
	"testing"

	docker "github.com/fsouza/go-dockerclient"
)

func TestDocker(t *testing.T) {
	endpoint := "unix:///Users/laisky/Library/Containers/com.docker.docker/Data/docker.sock"
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

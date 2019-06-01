package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

// Container contains the information about the container.
type Container struct {
	ID       string
	Port     string
	User     string
	Pass     string
	Database string
}

// StartPostgres runs a postgres container to execute commands.
func StartPostgres(log *log.Logger) (*Container, error) {
	user := "postgres"
	pass := "postgres"

	cmd := exec.Command("docker", "run", "--env", "POSTGRES_USER="+user, "--env", "POSTGRES_PASSWORD="+pass, "-P", "-d", "postgres:11-alpine")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("starting container: %v", err)
	}

	id := out.String()[:12]
	log.Println("DB ContainerID:", id)

	cmd = exec.Command("docker", "inspect", id)
	out.Reset()
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("inspect container: %v", err)
	}

	var doc []struct {
		NetworkSettings struct {
			Ports struct {
				TCP5432 []struct {
					HostPort string `json:"HostPort"`
				} `json:"5432/tcp"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		return nil, fmt.Errorf("decoding json: %v", err)
	}

	c := Container{
		ID:       id,
		Port:     doc[0].NetworkSettings.Ports.TCP5432[0].HostPort,
		User:     user,
		Pass:     pass,
		Database: "postgres",
	}

	log.Println("DB Port:", c.Port)

	return &c, nil
}

// StopPostgres stops and removes the specified container.
func StopPostgres(log *log.Logger, c *Container) error {
	if err := exec.Command("docker", "stop", c.ID).Run(); err != nil {
		return err
	}
	log.Println("Stopped:", c.ID)

	if err := exec.Command("docker", "rm", c.ID, "-v").Run(); err != nil {
		return err
	}
	log.Println("Removed:", c.ID)

	return nil
}

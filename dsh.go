package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dotcloud/docker/pkg/units"
	"github.com/fsouza/go-dockerclient"
)

var (
	builtins = map[string]func([]string) error{
		"exit": exit,
		"ps":   ps,
		"kill": kill,
		"ls":   ls,
		"run":  run,
	}

	client *docker.Client
)

func init() {
	var err error

	if client, err = docker.NewClient(os.Getenv("DOCKER_HOST")); err != nil {
		log.Fatal(err)
	}
}

func exit(args []string) error {
	code := 0

	if len(args) == 1 {
		c, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}
		code = c
	}

	os.Exit(code)

	return nil
}

func ps(args []string) error {
	containers, err := client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 10, 1, 3, ' ', 0)
	fmt.Fprintf(w, "ID\tIMAGE\tCMD\n")

	for _, c := range containers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.ID[0:5], c.Image, c.Command)
	}

	w.Flush()

	return nil
}

func kill(args []string) error {
	return client.KillContainer(docker.KillContainerOptions{ID: args[0]})
}

func ls(args []string) error {
	images, err := client.ListImages(false)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 10, 1, 3, ' ', 0)
	fmt.Fprintf(w, "ID\tSIZE\tDATE\tNAME\n")

	for _, i := range images {
		if strings.Contains(i.RepoTags[0], "<none>") {
			continue
		}

		name := strings.Split(i.RepoTags[0], ":")[0]

		t := time.Unix(i.Created, 0).Format("Jan 02")

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", i.ID[:5], units.HumanSize(i.VirtualSize), t, name)
	}

	w.Flush()

	return nil
}

func run(args []string) error {
	d := args[len(args)-1] == "&"

	if d {
		args = args[:len(args)-1]
	}

	c, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			AttachStdin:  true,
			AttachStdout: !d,
			AttachStderr: !d,
			Tty:          true,
			OpenStdin:    true,
			Cmd:          args[1:],
			Image:        args[0][2:],
		},
	})
	if err != nil {
		return err
	}

	if err := client.StartContainer(c.ID, &docker.HostConfig{}); err != nil {
		return err
	}

	if d {
		return nil
	}

	connected := make(chan struct{})
	go func() {
		connected <- (<-connected)
		log.Println("attached")
		fmt.Fprintf(os.Stdout, "# ") // fake first prompt
	}()

	if err := client.AttachToContainer(docker.AttachToContainerOptions{
		Container:    c.ID,
		InputStream:  os.Stdin,
		OutputStream: os.Stdout,
		ErrorStream:  os.Stderr,
		Stream:       true,
		Stdin:        true,
		Stdout:       true,
		Stderr:       true,
		RawTerminal:  true,
		Success:      connected,
	}); err != nil {
		return err
	}

	log.Println("detached")

	return nil
}

func prompt() {
	fmt.Fprintf(os.Stdout, "> ")
}

func main() {
	s := bufio.NewScanner(os.Stdin)

	fmt.Fprintln(os.Stdout, "the shell for the 2000nds")

	for {
		prompt()
		if !s.Scan() {
			break
		}

		tokens := strings.Split(s.Text(), " ")

		if len(tokens[0]) > 2 && tokens[0][:2] == "./" {
			if err := run(tokens); err != nil {
				log.Fatal(err)
			}
			continue
		}

		if b, exists := builtins[tokens[0]]; exists {
			if err := b(tokens[1:]); err != nil {
				log.Fatal(err)
			}
			continue
		}

		cmd := exec.Command(tokens[0], tokens[1:]...)

		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
	}
}

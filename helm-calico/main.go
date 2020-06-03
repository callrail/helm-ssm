package main

import (
	"fmt"
	"os"
	//"os/exec"
	//"github.com/pkg/errors"
)

const (
)

func main() {
	args := os.Args[1:]

	// TODO make template just spit out the manifest
	// if the command is not "install", "upgrade", or "template", pass the args to the regular helm command
	var install bool
	if (args[0] == "-n" || args[0] == "--namespace") { // order will be different if first arg is namespace flag
		install = checkForInstall(args, 2)
	} else {
		install = checkForInstall(args, 0)
	}
	if !install {
		if err := helmCommand(args); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	fmt.Println("it's a installll")
}

func checkForInstall(args []string, index int) bool {
	switch args[index] {
	case "install":
		return true
	case "upgrade":
		return true
	case "template":
		return true
	default:
		return false
	}
}

func helmCommand(args []string) error {
	fmt.Println("Running the helm command...")
	//helmCmd := exec.Command("helm", args...)
	//out, err := helmCmd.CombinedOutput()
	//fmt.Println(string(out))
	//if err != nil {
	//	return errors.Wrap(err, "error running helm command")
	//}
	return nil
}

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"github.com/pkg/errors"
)

const (
	SSM_FORMAT = `{{ssm\s+(\S+)\s?}}`
	SSM_PATH_FORMAT = `{{ssm-path\s+(\S+)\s?}}`
)

type ssmParams struct {
	params []string
	pathParams []string
}

func main() {
	args := os.Args[1:]

	// if the command is not "install" or "upgrade", or just a single command (no value files is a given in this case), pass the args to the regular helm command
	var install bool
	if (len(args) < 1) {
		install = false
	} else if (args[0] == "-n" || args[0] == "--namespace") { // check if first arg is namespace flag
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

	// Get the values files
	valueFiles := getValueFiles(args)
	// concatenate the values files
	mergedValues, err := mergeValueFiles(valueFiles)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// find the lines that match ssm keywords and get params
	ssmParams, err := getParams(mergedValues)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("pathParams:")
	for _, path := range ssmParams.pathParams {
		fmt.Println(path)
	}
	fmt.Println("params:")
	for _, param := range ssmParams.params {
		fmt.Println(param)
	}

}

func getValueFiles(args []string) []string {
	valueFiles := []string{}
	for i, arg := range args {
		if (arg == "-f" || arg == "--values") {
			if !(i + 1 == len(args)) {
				valueFiles = append(valueFiles, args[i+1])
			}
		}
	}
	return valueFiles
}

func mergeValueFiles(valueFiles []string) ([]string, error) {
	mergedValues := []string{}
	for _, valueFile := range valueFiles {
		lines, err := readLines(valueFile)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading value file %s", valueFile)
		}
		mergedValues = append(mergedValues, lines...)
	}
	return mergedValues, nil
}

func readLines(valueFile string) ([]string, error) {
	lines := []string{}
	file, err := os.Open(valueFile)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n') // TODO don't add blank lines
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return nil, err
			}
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func getParams(values []string) (*ssmParams, error) {
	reSSM := regexp.MustCompile(SSM_FORMAT)
	reSSMPath := regexp.MustCompile(SSM_PATH_FORMAT)
	ssmParams := ssmParams{params: []string{}, pathParams: []string{}}
	for _, line := range values {
		if reSSMPath.MatchString(line) {
			paramSubmatch := reSSMPath.FindStringSubmatch(line)
			if len(paramSubmatch) < 2 {
				return nil, errors.New(fmt.Sprintf("format error in line %s", line))
			}
			ssmParams.pathParams = append(ssmParams.pathParams, paramSubmatch[1])
		} else if reSSM.MatchString(line) {
			paramSubmatch := reSSM.FindStringSubmatch(line)
			if len(paramSubmatch) < 2 {
				return nil, errors.New(fmt.Sprintf("format error in line %s", line))
			}
			ssmParams.params = append(ssmParams.params, paramSubmatch[1])
		}
	}
	return &ssmParams, nil
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
	helmCmd := exec.Command("helm", args...)
	out, err := helmCmd.CombinedOutput()
	fmt.Println(string(out))
	if err != nil {
		return errors.Wrap(err, "error running helm command")
	}
	return nil
}

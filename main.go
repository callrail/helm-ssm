package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/pkg/errors"
)

const (
	SSM_FORMAT = `{{ssm\s+(\S+)\s?}}`
	SSM_PATH_FORMAT = `{{ssm-path\s+(\S+)\s?}}`
)

type controller struct {
	awsClient *ssm.SSM
}

func main() {
	args := os.Args[1:]
	c := &controller{}

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
	valueFiles, newArgs := pullValueFiles(args)

	// concatenate the values files
	mergedValues, err := mergeValueFiles(valueFiles)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// find the lines that match ssm keywords, go get the values, and replace them
	newValues, err := c.findAndReplace(mergedValues)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := helmCommandWithNewValues(newValues, newArgs); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (c *controller) initializeAWSClient() error {
	sess, err := session.NewSessionWithOptions(session.Options{
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{},
	})
	if err != nil {
		return err
	}
	c.awsClient = ssm.New(sess)
	return nil
}

func pullValueFiles(args []string) ([]string, []string) {
	valueFiles := []string{}
	newArgs := []string{}
	lastWasValuesFileFlag := false
	for i, arg := range args {
		if (arg == "-f" || arg == "--values") {
			if !(i + 1 == len(args)) {
				valueFiles = append(valueFiles, args[i+1])
			}
			lastWasValuesFileFlag = true
		} else if !lastWasValuesFileFlag {
			newArgs = append(newArgs, arg)
			lastWasValuesFileFlag = false
		} else {
			lastWasValuesFileFlag = false
		}
	}
	return valueFiles, newArgs
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
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return nil, err
			}
		}
		if (line != "\n") {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func (c *controller) findAndReplace(values []string) ([]string, error) {
	newValues := []string{}
	reSSM := regexp.MustCompile(SSM_FORMAT)
	reSSMPath := regexp.MustCompile(SSM_PATH_FORMAT)
	for _, line := range values {
		if reSSMPath.MatchString(line) {
			// extract the value of the paramater or path name
			paramSubmatch := reSSMPath.FindStringSubmatch(line)
			if len(paramSubmatch) < 2 {
				return nil, errors.New(fmt.Sprintf("format error in line %s", line))
			}

			newLine, err := c.replaceWithSSMPath(line, paramSubmatch[1])
			if err != nil {
				return nil, err
			}
			newValues = append(newValues, newLine)
		} else if reSSM.MatchString(line) {
			paramSubmatch := reSSM.FindStringSubmatch(line)
			if len(paramSubmatch) < 2 {
				return nil, errors.New(fmt.Sprintf("format error in line %s", line))
			}
			newLine, err := c.replaceWithSSMParameter(line, paramSubmatch[1])
			if err != nil {
				return nil, err
			}
			newValues = append(newValues, newLine)
		} else {
			newValues = append(newValues, line)
		}
	}
	return newValues, nil
}

func (c *controller) replaceWithSSMParameter(line string, path string) (string, error) {
	// if awsClient is not yet initialized, initialize it
	if c.awsClient == nil {
		if err := c.initializeAWSClient(); err != nil {
			return "", errors.Wrap(err, "error initializing AWS client")
		}
	}

	param, err := c.awsClient.GetParameter(
		&ssm.GetParameterInput{
			Name: &path,
			WithDecryption: aws.Bool(true),
		},
	)
	if err != nil {
		return "", errors.Wrapf(err, "error getting paramater %s from AWS", path)
	}
	
	line, err = constructReplacementLine(line, *param.Parameter.Value)
	if err != nil {
		return "", err
	}

	return line, nil
}

func (c *controller) replaceWithSSMPath(line string, path string) (string, error) {
	// if awsClient is not yet initialized, initialize it
	if c.awsClient == nil {
		if err := c.initializeAWSClient(); err != nil {
			return "", errors.Wrap(err, "error initializing AWS client")
		}
	}

	//get all parameters starting with that path
	params := map[string]string{}
	if err := c.awsClient.GetParametersByPathPages(
		&ssm.GetParametersByPathInput{
			Path: &path,
			Recursive: aws.Bool(true),
			WithDecryption: aws.Bool(true),
		},
		func(page *ssm.GetParametersByPathOutput, lastPage bool) bool {
			for _, param := range page.Parameters {
				key := (*param.Name)[len(path)+1:] // trim out the path
				params[key] = *param.Value
			}
			return true
		},
	); err != nil {
		return "", errors.Wrapf(err, "error getting paramaters from path %s from AWS", path)
	}

	paramDict, err := json.Marshal(params)
	if err != nil {
		return "", errors.Wrap(err, "error marshalling parameters into values")
	}

	line, err = constructReplacementLine(line, string(paramDict))
	if err != nil {
		return "", err
	}
	return line, nil
}

func constructReplacementLine(line, newValue string) (string, error) {
	// contruct the new line for the values file. keep everything until and including the colon
	colon := strings.Index(line, ":")
	if (colon == -1) {
		return "", errors.New(fmt.Sprintf("format error in line %s", line))
	}
	return fmt.Sprintf("%s %s\n", line[:colon+1], newValue), nil
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

func helmCommandWithNewValues(values []string, args []string) error {
	tempFile := fmt.Sprintf("%s-temp-values.yaml", time.Now().Format("20060102150405"))

	// write temp file
	f, err := os.OpenFile(tempFile,os.O_APPEND|os.O_CREATE|os.O_WRONLY,0644)
	if err != nil {
		return errors.Wrap(err, "error writing temp values file")
	}
	writer := bufio.NewWriter(f)
	for _, line := range values {
		_, err := writer.WriteString(line)
		if err != nil {
			return errors.Wrap(err, "error writing temp values file")
		}
	}
	writer.Flush()
	f.Close()

	// helm command
	args = append(args, "-f", tempFile)
	if err = helmCommand(args); err != nil {
		// delete the file
		if deleteErr := os.Remove(tempFile); deleteErr != nil {
			return errors.Wrapf(err, "error running helm command, and could not delete temp values file %s", tempFile)
		}
		return err
	}

	// delete the temp file
	if err := os.Remove(tempFile); err != nil {
		return errors.Wrapf(err, "error deleting temp values file %s", tempFile)
	}

	return nil
}

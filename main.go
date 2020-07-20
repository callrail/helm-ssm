package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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
	SSM_PATH_PREFIX_FORMAT = `{{ssm-path-prefix\s+(\S+)\s?}}`
	LIST_ITEM_FORMAT = `^\s{0,}-\s(\S+)\n?$`
	END_FORMAT = `^\s{0,}{{\s?end\s?}}`
	COMMENT_FORMAT = `^\s{0,}#.*`
)

type controller struct {
	awsClient *ssm.SSM
	opts      options
}

type options struct {
	keepTempValuesFile bool
}

func main() {
	args := os.Args[1:]
	c := &controller{
		opts: options{keepTempValuesFile: false},
	}

	// if the command is not "install" or "upgrade", or just a single command (no value files is a given in this case), pass the args to the regular helm command
	var install bool
	if (len(args) < 1) {
		install = false
	} else if (args[0] == "-n" || args[0] == "--namespace") { // order will be different if first arg is namespace flag
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

	args = c.pullNonHelmArgs(args)

	valueFiles, newArgs := pullValueFiles(args)
	mergedValues, err := mergeValueFiles(valueFiles)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// find the lines that match ssm keywords, go get the values, and replace them
	newValues, changed, err := c.findAndReplace(mergedValues)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// if there was nothing replaced, no need to write a new temp values file
	if !changed {
		if err := helmCommand(args); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		if err := c.helmCommandWithNewValues(newValues, newArgs); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
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

func (c *controller) pullNonHelmArgs(args []string) []string {
	index := -1
	for i, arg := range args {
		if arg == "--keep-temp-values-file" {
			c.opts.keepTempValuesFile = true
			index = i
		}
	}

	if c.opts.keepTempValuesFile {
		args[index] = args[len(args) - 1]
		args = args[:len(args) - 1]
	}

	return args
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

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		newLine := scanner.Text()
		// if the line is empty or commented out, don't add
		if ((newLine != "") && (newLine != "\n") && !regexp.MustCompile(COMMENT_FORMAT).Match([]byte(newLine))) {
			lines = append(lines, newLine)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// returns a slice of the new value lines, a bool indicating whether or not a replacement occured, and an error
func (c *controller) findAndReplace(values []string) ([]string, bool, error) {
	newValues := []string{}
	var changed bool
	reSSM := regexp.MustCompile(SSM_FORMAT)
	reSSMPath := regexp.MustCompile(SSM_PATH_FORMAT)
	reSSMPathPrefix := regexp.MustCompile(SSM_PATH_PREFIX_FORMAT)
	linesToDelete := []int{}
	for i, line := range values {
		// do ssm-path-prefix first
		if loc := reSSMPathPrefix.FindStringSubmatchIndex(line); loc != nil {  // returns [starting index of regex, end index of regex, start index of submatch, end index of submatch]
			if len(loc) < 4 {
				return nil, changed, errors.New(fmt.Sprintf("format error in line %s", line))
			}
			changed = true
			// in this case, we want to grab all subsequent lines until we see {{end}}
			newLine, numLinesToDelete, err := c.replaceWithSSMPathPrefix(line, loc, values[i+1:])
			if err != nil {
				return nil, changed, err
			}
			// Mark the lines we need to delete after (up until {{end}}
			for j := i + 1; j <= (i + numLinesToDelete); j++ {
				linesToDelete = append(linesToDelete, j)
			}
			newValues = append(newValues, newLine)
		} else if loc := reSSMPath.FindStringSubmatchIndex(line); loc != nil {
			if len(loc) < 4 {
				return nil, changed, errors.New(fmt.Sprintf("format error in line %s", line))
			}
			changed = true
			newLine, err := c.replaceWithSSMPath(line, loc)
			if err != nil {
				return nil, changed, err
			}
			newValues = append(newValues, newLine)
		} else if loc := reSSM.FindStringSubmatchIndex(line); loc != nil {
			if len(loc) < 4 {
				return nil, changed, errors.New(fmt.Sprintf("format error in line %s", line))
			}
			changed = true
			newLine, err := c.replaceWithSSMParameter(line, loc)
			if err != nil {
				return nil, changed, err
			}
			newValues = append(newValues, newLine)
		} else {
			newValues = append(newValues, line)
		}
	}

	// Delete out linesToDelete
	for _, lineNumber := range linesToDelete {
		newValues[lineNumber] = ""
	}

	return newValues, changed, nil
}

func (c *controller) replaceWithSSMParameter(line string, locationMatch []int) (string, error) {
	paramPath := line[locationMatch[2]:locationMatch[3]]

	// if awsClient is not yet initialized, initialize it
	if c.awsClient == nil {
		if err := c.initializeAWSClient(); err != nil {
			return "", errors.Wrap(err, "error initializing AWS client")
		}
	}

	param, err := c.awsClient.GetParameter(
		&ssm.GetParameterInput{
			Name: &paramPath,
			WithDecryption: aws.Bool(true),
		},
	)
	if err != nil {
		return "", errors.Wrapf(err, "error getting paramater %s from AWS", paramPath)
	}
	
	line = constructReplacementLine(line, locationMatch, *param.Parameter.Value)

	return line, nil
}

func (c *controller) replaceWithSSMPath(line string, locationMatch []int) (string, error) {
	paramPath := line[locationMatch[2]:locationMatch[3]]

	if c.awsClient == nil {
		if err := c.initializeAWSClient(); err != nil {
			return "", errors.Wrap(err, "error initializing AWS client")
		}
	}

	params := map[string]string{}
	if err := c.awsClient.GetParametersByPathPages(
		&ssm.GetParametersByPathInput{
			Path: &paramPath,
			Recursive: aws.Bool(true),
			WithDecryption: aws.Bool(true),
		},
		func(page *ssm.GetParametersByPathOutput, lastPage bool) bool {
			for _, param := range page.Parameters {
				key := (*param.Name)[len(paramPath)+1:] // trim out the path
				params[key] = *param.Value
			}
			return true
		},
	); err != nil {
		return "", errors.Wrapf(err, "error getting paramaters from path %s from AWS", paramPath)
	}

	paramDict, err := json.Marshal(params)
	if err != nil {
		return "", errors.Wrap(err, "error marshalling parameters into values")
	}

	line = constructReplacementLine(line, locationMatch, string(paramDict))
	return line, nil
}

// returns replacement line, number of lines to delete, and error
func (c *controller) replaceWithSSMPathPrefix(line string, locationMatch []int, values []string) (string, int, error) {
	prefix := line[locationMatch[2]:locationMatch[3]]
	paramPaths := []string{}

	// read in lines from values and grab paramPaths until we see {{end}}
	lineCount := 0
	for i, l := range values {
		lineCount = i+1
		if match := regexp.MustCompile(LIST_ITEM_FORMAT).FindStringSubmatch(l); match != nil {
			if (len(match) < 2) {
				return "", 0, errors.New(fmt.Sprintf("format error in line %s", l))
			}
			paramPaths = append(paramPaths, fmt.Sprintf("%s%s", prefix, match[1]))
		} else if regexp.MustCompile(END_FORMAT).Match([]byte(l)) {
			break
		}
		// if we never receive an {{end}}, throw an error
		if (i == (len(values) - 1)) {
			return "", 0, errors.New("error: no {{end}} found")
		}
	}

	if c.awsClient == nil {
		if err := c.initializeAWSClient(); err != nil {
			return "", 0, errors.Wrap(err, "error initializing AWS client")
		}
	}

	allParams := []map[string]string{}
	for _, paramPath := range paramPaths {
		params := map[string]string{}
		if err := c.awsClient.GetParametersByPathPages(
			&ssm.GetParametersByPathInput{
				Path: &paramPath,
				Recursive: aws.Bool(true),
				WithDecryption: aws.Bool(true),
			},
			func(page *ssm.GetParametersByPathOutput, lastPage bool) bool {
				for _, param := range page.Parameters {
					key := (*param.Name)[len(paramPath)+1:] // trim out the path
					params[key] = *param.Value
				}
				return true
			},
		); err != nil {
			return "", 0, errors.Wrapf(err, "error getting paramaters from path %s from AWS", paramPath)
		}
		allParams = append(allParams, params)
	}

	paramDict, err := json.Marshal(allParams)
	if err != nil {
		return "", 0, errors.Wrap(err, "error marshalling parameters into values")
	}

	
	line = constructReplacementLine(line, locationMatch, string(paramDict))
	return line, lineCount, nil
}

func constructReplacementLine(line string, location []int, newValue string) string {
	return line[:location[0]] + newValue + "\n"
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
	if ((args[0] == "--help") || (args[0] == "-h")) {
		fmt.Println("helm ssm usage:")
		fmt.Println("\thelm ssm [command] [--keep-temp-values-file] [helm args...]")
		fmt.Println("Flags:")
		fmt.Println("\t--keep-temp-values-file\t\t\tIf true, don't clean up the temporary values file populated with ssm values from the current directory")
		fmt.Println("\n\nHelm Usage:")
	}
	helmCmd := exec.Command("helm", args...)
	out, err := helmCmd.CombinedOutput()
	fmt.Println(string(out))
	if err != nil {
		return errors.Wrap(err, "error running helm command")
	}
	return nil
}

func (c *controller) helmCommandWithNewValues(values []string, args []string) error {
	tempFile := fmt.Sprintf("%s-temp-values.yaml", time.Now().Format("20060102150405"))

	f, err := os.OpenFile(tempFile,os.O_APPEND|os.O_CREATE|os.O_WRONLY,0644)
	if err != nil {
		return errors.Wrap(err, "error writing temp values file")
	}
	writer := bufio.NewWriter(f)
	for _, line := range values {
		if line != "" {
			_, err := writer.WriteString(line + "\n")
			if err != nil {
				return errors.Wrap(err, "error writing temp values file")
			}
		}
	}
	writer.Flush()
	f.Close()

	args = append(args, "-f", tempFile)
	if err = helmCommand(args); err != nil {
		if !c.opts.keepTempValuesFile {
			if deleteErr := os.Remove(tempFile); deleteErr != nil {
				return errors.Wrapf(err, "error running helm command, and could not delete temp values file %s", tempFile)
			}
		}
		return err
	}

	if !c.opts.keepTempValuesFile {
		if err := os.Remove(tempFile); err != nil {
			return errors.Wrapf(err, "error deleting temp values file %s", tempFile)
		}
	}

	return nil
}

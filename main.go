package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
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
	awsClient *ssm.Client
	opts      options
}

type options struct {
	keepTempValuesFile bool
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
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
			return err
		}
		return nil
	}

	args = c.pullNonHelmArgs(args)

	valueFiles, newArgs := pullValueFiles(args)

	// Resolve SSM directives in EACH value file independently and hand helm one
	// `-f` per file, preserving the original order, so helm performs its native
	// multi-file merge (deep-merge maps, later file wins) across all of them.
	helmArgs, tempFiles, err := c.resolveValueFiles(valueFiles, newArgs)
	if err != nil {
		cleanupTempFiles(tempFiles)
		return err
	}

	err = helmCommand(helmArgs)
	if !c.opts.keepTempValuesFile {
		cleanupTempFiles(tempFiles)
	}
	return err
}

func (c *controller) initializeAWSClient() error {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = stscreds.StdinTokenProvider
		}),
	)
	if err != nil {
		return err
	}
	c.awsClient = ssm.NewFromConfig(cfg)
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

// resolveValueFiles resolves SSM directives in each value file independently and
// appends one `-f` per file to helmArgs — a temp file if anything was replaced,
// the original path otherwise. Order is preserved so helm's multi-file merge
// precedence (later file wins) holds. Temp files are returned for cleanup, even on error.
func (c *controller) resolveValueFiles(valueFiles, baseArgs []string) ([]string, []string, error) {
	helmArgs := baseArgs
	tempFiles := []string{}
	for i, valueFile := range valueFiles {
		lines, err := readLines(valueFile)
		if err != nil {
			return nil, tempFiles, errors.Wrapf(err, "error reading value file %s", valueFile)
		}
		newValues, changed, err := c.findAndReplace(lines)
		if err != nil {
			return nil, tempFiles, err
		}
		if changed {
			tempFile, err := writeTempValuesFile(newValues, i)
			if err != nil {
				return nil, tempFiles, err
			}
			tempFiles = append(tempFiles, tempFile)
			helmArgs = append(helmArgs, "-f", tempFile)
		} else {
			helmArgs = append(helmArgs, "-f", valueFile)
		}
	}
	return helmArgs, tempFiles, nil
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
		context.Background(),
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

	params, err := c.getParametersByPath(paramPath)
	if err != nil {
		return "", errors.Wrapf(err, "error getting paramaters from path %s from AWS", paramPath)
	}

	paramDict, err := json.Marshal(params)
	if err != nil {
		return "", errors.Wrap(err, "error marshalling parameters into values")
	}

	line = constructReplacementLine(line, locationMatch, string(paramDict))
	return line, nil
}

// getParametersByPath fetches every parameter under paramPath (recursively, decrypted),
// paging through results, and returns them keyed by their name with paramPath trimmed off.
func (c *controller) getParametersByPath(paramPath string) (map[string]string, error) {
	params := map[string]string{}
	paginator := ssm.NewGetParametersByPathPaginator(c.awsClient, &ssm.GetParametersByPathInput{
		Path: &paramPath,
		Recursive: aws.Bool(true),
		WithDecryption: aws.Bool(true),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}
		for _, param := range page.Parameters {
			key := (*param.Name)[len(paramPath)+1:] // trim out the path
			params[key] = *param.Value
		}
	}
	return params, nil
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
		params, err := c.getParametersByPath(paramPath)
		if err != nil {
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
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", string(out))
		return errors.Wrap(err, "error running helm command")
	}
	fmt.Println(string(out))
	return nil
}

// writeTempValuesFile writes resolved value lines to a uniquely-named temp file
// in the current working directory and returns its path. The idx suffix keeps
// names unique when several files are written within the same second (the
// timestamp alone is 1s granularity). The `-temp-values.yaml` suffix is retained
// so existing cleanup globs (helm_deploy.sh, .gitignore) keep matching.
func writeTempValuesFile(values []string, idx int) (string, error) {
	tempFile := fmt.Sprintf("%s-%d-temp-values.yaml", time.Now().Format("20060102150405"), idx)

	f, err := os.OpenFile(tempFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", errors.Wrap(err, "error writing temp values file")
	}
	writer := bufio.NewWriter(f)
	for _, line := range values {
		if line != "" {
			if _, err := writer.WriteString(line + "\n"); err != nil {
				f.Close()
				return "", errors.Wrap(err, "error writing temp values file")
			}
		}
	}
	writer.Flush()
	f.Close()

	return tempFile, nil
}

// cleanupTempFiles removes the temp value files created during resolution.
// Best-effort: a leftover temp file is harmless (it's gitignored, and
// helm_deploy.sh also sweeps `*-temp-values.yaml`), so removal errors are not
// treated as fatal.
func cleanupTempFiles(tempFiles []string) {
	for _, tempFile := range tempFiles {
		_ = os.Remove(tempFile)
	}
}

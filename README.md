# helm-ssm
A low-dependency tool used to retrieves and injects secrets from AWS SSM Parameter Store.
Modified from: https://github.com/totango/helm-ssm


## Installation
```bash
$ helm plugin install https://github.com/callrail/helm-ssm
```
For username, enter your GitHub username.

For password, enter your **Personal Access Token**. (You have probably created one already. See step #12 [here](https://github.com/callrail/setup).)

## Updating
```
$ helm plugin update ssm
```

## Testing
```
$ go run main.go install testing ./tests/testchart/ -f tests/testchart/override-values.yaml --dry-run --debug
```


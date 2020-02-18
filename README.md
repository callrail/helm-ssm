# helm-ssm
A low-dependency tool used to retrieves and injects secrets from AWS SSM Parameter Store.
Modified from: https://github.com/totango/helm-ssm


## Installation
```bash
$ helm plugin install https://github.com/callrail/helm-ssm
```

## Testing
```
$ ./ssm.sh install helm-ssm-test tests/testchart/ --debug --dry-run -f tests/testchart/values.yaml
```

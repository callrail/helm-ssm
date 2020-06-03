<a href="https://github.com/callrail/helm-ssm/actions"><img alt="GitHub Actions status" src="https://github.com/callrail/helm-ssm/workflows/Build%20and%20Release/badge.svg"></a>

# helm-ssm
A tool used to retrieve and inject secrets from AWS SSM Parameter Store into helm value files.

Idea modified from: https://github.com/totango/helm-ssm


## Installation
```bash
$ helm plugin install https://github.com/callrail/helm-ssm
```

## Updating
```
$ helm plugin update ssm
```

## Usage
In any **non-default** values file, replace values of secrets with ssm keywords `ssm`, `ssm-path`, and `ssm-path-prefix` as shown below.
#### Single Parameter
Replace a value-file value with a value from SSM Parameter Store:
```
mySecret: {{ssm <my-ssm-parameter-name>}}
```
Then run your helm install/update command as usual but with `helm ssm` instead of just `helm`.

For example,
```
$ helm ssm install my-release my-chart -f my-values-file.yaml
```
**Note:** You will need to run your helm command using credentials with access to SSM in the AWS account in which the parameter lives.

#### Multiple Parameters under a Single Path
You can also include a map of key/value pairs by specifying a path that holds multiple parameters.

For example, say you have the following parameters in SSM:
```
/prod-config/example/secret-key-1  =>  "value-1"
/prod-config/example/secret-key-2  =>  "value-2"
/prod-config/example/secret-key-3  =>  "value-3"
```
Then the following values file will result in a dictionary of the key/value pairs.
```
myConfig: {{ssm-path /prod-config/example}}

 => becomes =>

myConfig: {secret-key-1: "value-1", secret-key-2: "value-2": secret-key-3: "value-3"}
```

#### Multiple Parameters under Multiple Paths sharing a common prefix
Let's say I want to include multiple parameter paths that have a common prefix. For example,
```
/prod-config/prod_hosts/host_1_key => "secret-value"
/prod-config/prod_hosts/host_2_key => "secret-value"

/prod-config/api_tokens/app_1_token => "secret-value"
/prod-config/api_tokens/app_2_token => "secret-value"
/prod-config/api_tokens/app_3_token => "secret-value"

/prod-config/database_urls/db_url => "secret-value"
```
Then the following values file will result in a list of dictionaries of the key/value pairs.
```
myConfig: {{ssm-path-prefix /prod-config/}}
  - prod_hosts
  - api_tokens
  - database_urls
{{end}}

 => becomes =>

myConfig:
  - {host_1_key: "secret-value", host_2_key: "secret-value"}
  - {app_1_token: "secret-value", app_2_token: "secret-value", app_3_token: "secret-value"}
  - {db_url: "secret-value"}
```

## Testing
This testing setup assumes you have the following parameters in SSM:
```
test-secret-value: (value can be anything)
/test-secret-group/value1: (value can be anything)
/test-secret-group/value2: (value can be anything)
/test-secret-group-2/config1/c1key1: (value can be anything)
/test-secret-group-2/config2/c2key1: (value can be anything)
/test-secret-group-2/config2/c2key2: (value can be anything)

...
(as many as you want under the path /test-secret-group/)
```
```
$ go run main.go install testing ./tests/testchart/ -f tests/testchart/override-values.yaml --dry-run --debug
```


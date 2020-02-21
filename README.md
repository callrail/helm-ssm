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

## Usage
In any non-default values file, replace values of secrets with ssm keywords `ssm` and `ssm-path` as shown below.
#### Single Parameter
Replace a value-file value with a value from SSM Parameter Store:
```
mySecret: {{ssm <my-ssm-parameter-name>}}
```
Then run your helm install/update command as usual but with `helm ssm` instead of just `helm`.

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

## Testing
```
$ go run main.go install testing ./tests/testchart/ -f tests/testchart/override-values.yaml --dry-run --debug
```


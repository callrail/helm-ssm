#HELM_PLUGIN_DIR="/Users/jessicatracy/fake-helm-plugin-dur/" # TODO
#
## cd to the plugin dir
#cd $HELM_PLUGIN_DIR
#
## get the version
#version="$(cat plugin.yaml | grep "version" | cut -d '"' -f 2)"
#
## find the OS and ARCH
#unameOut="$(uname -s)"
#
#case "${unameOut}" in
#    Linux*)     os=Linux;;
#    Darwin*)    os=Darwin;;
#    CYGWIN*)    os=Cygwin;;
#    MINGW*)     os=windows;;
#    *)          os="UNKNOWN:${unameOut}"
#esac
#
#arch=`uname -m`
#
## set the url of the binary
##url="https://github.com/callrail/helm-ssm/releases/download/v${version}/helm-ssm${version}_${os}_${arch}"
##url="https://github.com/callrail/helm-ssm/releases/download/v0.1.0/helm-ssm"
##releases_url="https://api.github.com/repos/callrail/helm-ssm/releases" # gets all releases
##url="https://api.github.com/repos/callrail/helm-ssm/releases/assets/18147422" # this is ID of my current asset but it's just json blob of it, not binary
## ???
#echo $url
#
## set the filename
#filename=`echo ${url} | sed -e "s/^.*\///g"`
#
## download the binary using curl or wget
#if [ -n $(command -v curl) ]
#then
#    #curl -sSL -H "Authorization: token $GITHUB_TOKEN" -O $url
#    curl -u jesstracy:$GITHUB_TOKEN -O $url # not working
#elif [ -n $(command -v wget) ]
#then
#    wget -q $url
#else
#    echo "Need curl or wget"
#    exit -1
#fi
#
## move binary into the bin dir
#rm -rf bin && mkdir bin && mv $filename bin/helm-ssm && chmod +x bin/helm-ssm

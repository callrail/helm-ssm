if [ -z "$HELM_PLUGIN_DIR" ]; then
  echo "HELM_PLUGIN_DIR is not set"
  exit 1
fi
# cd to the plugin dir
cd $HELM_PLUGIN_DIR

# get the version
version="$(cat plugin.yaml | grep "version" | cut -d '"' -f 2)"
version="v${version}"

# find the OS and ARCH
unameOut="$(uname -s)"

case "${unameOut}" in
    Linux*)     os=Linux;;
    Darwin*)    os=Darwin;;
    CYGWIN*)    os=Cygwin;;
    MINGW*)     os=windows;;
    *)          os="UNKNOWN:${unameOut}"
esac

arch=`uname -m`

# set the url of the binary
url="https://github.com/callrail/helm-ssm/releases/download/${version}/helm-ssm_${version}_${os}_${arch}"
echo "url: $url"

# set the filename
filename=`echo ${url} | sed -e "s/^.*\///g"`
echo "filename: $filename"

# download the binary using curl or wget
if [ -n $(command -v curl) ]
then
    curl -sSL -O $url
elif [ -n $(command -v wget) ]
then
    wget -q $url
else
    echo "Need curl or wget"
    exit -1
fi

# move binary into the bin dir
rm -rf bin && mkdir bin && mv $filename bin/helm-ssm && chmod +x bin/helm-ssm

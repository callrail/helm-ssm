# cd to the plugin dir
cd $HELM_PLUGIN_DIR

# get the version
version="$(cat plugin.yaml | grep "version" | cut -d '"' -f 2)"

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

# set the url of the tar.gz
url="https://github.com/callrail/helm-ssm/releases/download/v${version}/helm-ssm_${version}_${os}_${arch}.tar.gz"
echo $url

# set the filename
filename=`echo ${url} | sed -e "s/^.*\///g"`

# download the archive using curl or wget
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

# extract the plugin binary into the bin dir
rm -rf bin && mkdir bin && tar xzvf $filename -C bin > /dev/null && rm -f $filename

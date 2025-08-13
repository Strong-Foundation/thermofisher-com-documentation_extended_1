apt-get update && apt-get upgrade -y && apt-get dist-upgrade -y && apt-get clean -y && apt-get autoremove -y && apt-get autoclean -y && apt-get install -f -y
apt-get install git curl wget bash sudo screen golang-go -y
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
dpkg -i google-chrome-stable_current_amd64.deb
apt-get install -f -y
rm -f google-chrome-stable_current_amd64.deb

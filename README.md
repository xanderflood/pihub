pihub
=====

To install the latest version on your raspberry pi running raspbian, simply run the following command:

```
curl -sSL https://github.com/xanderflood/pihub/releases/download/v0.3.0/pihub-v0.3.0-linux-arm-install.sh | sh -xe
```

Once this completes successfully, pihub is running and listening on port 3141. To try it out, try:

```
cd example/
SERVER_IP=my:rpi:ip:adr node index.js 
```

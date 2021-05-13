GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build --ldflags "-s -X main.user=$P_USER -X main.passwd=$P_PASSWD -X main.proxyAddr=$P_ADDR"

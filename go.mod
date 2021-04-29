module github.com/telepresenceio/telepresence/v2

go 1.15

require (
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5
	github.com/blang/semver v3.5.0+incompatible
	github.com/datawire/ambassador v1.12.3-0.20210401195424-3d91930ec3fd
	github.com/datawire/dlib v1.2.1
	github.com/docker/docker v1.4.2-0.20200203170920-46ec8731fbce
	github.com/godbus/dbus/v5 v5.0.4-0.20201218172701-b3768b321399
	github.com/google/go-cmp v0.5.0
	github.com/google/uuid v1.1.2
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/miekg/dns v1.1.35
	github.com/natefinch/npipe v0.0.0-20160621034901-c1b8fa8bdcce
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/pkg/errors v0.9.1
	github.com/sethvargo/go-envconfig v0.3.2
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/telepresenceio/telepresence/rpc/v2 v2.2.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/net v0.0.0-20210410081132-afb366fc7cd1
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9 // indirect
	golang.org/x/sys v0.0.0-20210403161142-5e06dd20ab57
	golang.zx2c4.com/wireguard v0.0.0-20210427022245-097af6e1351b
	golang.zx2c4.com/wireguard/windows v0.3.11
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20200806141610-86f49bd18e98 // indirect
	google.golang.org/grpc v1.34.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver v0.18.8 // indirect
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v0.18.8
	k8s.io/kubectl v0.18.8 // indirect
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/Azure/go-autorest v10.8.1+incompatible => github.com/Azure/go-autorest v13.3.2+incompatible

replace github.com/telepresenceio/telepresence/rpc/v2 => ./rpc

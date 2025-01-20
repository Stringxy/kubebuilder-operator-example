# kubebuilder-operator-example
XyDaemonset controller built using kubebuilder.Deploy pods on each node in the cluster by configuring images, number of replicas, and startup commands, and achieve scaling based on the number of replicas. 

使用kubebuilder构建的XyDaemonset控制器,通过配置镜像、副本数、启动命令，在集群每个节点部署node，并实现根据副本数扩缩容
## 环境 Environment

- kubebuilder: v4.4.0
- kubernetes api: v1.32.0

---

## 项目初始化
### Create a kubebuilder project, which requires an empty folder

```bash
# 初始化项目骨架
kubebuilder init --domain xy.io

# 初始化 CRD，两个确认都是 y
kubebuilder create api --group xytest --version v1 --kind XyDaemonset
```


## 定义CRD参数及结构
### Open project with IDE and edit api/v1/xydaemonset_types.go
`api/v1/xydaemonset_types.go` 定义了 Xydaemonset 的 CRD，执行命令如下命令生成 CRD 定义的 yaml 文件，文件位置在 `config/crd/bases/xytest.xy.io_xydaemonsets.yaml`

```bash
make manifests
```

之后使用 kubectl 将此 CRD 定义 apply 到 k8s 上

Use kubectl command to apply this CRD definition to k8s
```bash
kubectl apply -f config/crd/bases/xytest.xy.io_xydaemonsets.yaml
```

## 编写Controller逻辑，并配置权限
### Edit controllers/xydaemonset_controller.go, add permissions to the controller
```bash
// Add permissions to the controller
// +kubebuilder:rbac:groups=xytest.xy.io,resources=xydaemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=xytest.xy.io,resources=xydaemonsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=xytest.xy.io,resources=xydaemonsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get

```
执行如下命令在本地运行 controller

run controller service locally
```bash
make run
```

### Build & install

```sh
make build
make docker-build
make docker-push
make deploy
```

## 示例参数说明
### Example description
```yaml
apiVersion: xytest.xy.io/v1
kind: XyDaemonset
metadata:
labels:
app.kubernetes.io/name: data
app.kubernetes.io/managed-by: kustomize
name: xydaemonset-busybox
namespace: xytest
spec:
command: ["sleep", "3600"] # 容器启动命令
image: busybox  #容器镜像
replicas: 1 #每个节点上的副本数
```

## XyDaemonsetStatus属性说明
### XyDaemonsetStatus description
- AvailableReplicas：当前运行中的副本数
- PodNames：当前运行中的Pod名称
- AutoScalingStatus：是否正在扩缩容
```bash
// XyDaemonsetStatus defines the observed state of XyDaemonset.
type XyDaemonsetStatus struct {
// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
// Important: Run "make" to regenerate code after modifying this file

	AvailableReplicas int      `json:"availableReplicas"`
	PodNames          []string `json:"podNames"`
	AutoScalingStatus string   `json:"autoScalingStatus"`
}
```
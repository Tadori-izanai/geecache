# 在 geecache 中使用 etcd 做注册发现

本文目标有两件事：

1. 先把 `etcd` 和它的 Go SDK 讲清楚，方便你建立基本认知。
2. 再把它落到当前仓库里，说明怎样把现在的“静态节点列表”改造成“基于 etcd 动态维护节点”。

本文基于当前代码结构：

- `Group` 是缓存命名空间，定义在 `internal/geecache/geecache.go`
- gRPC 服务端在 `internal/geecache/grpc/server.go`
- 一致性哈希选点在 `internal/geecache/grpc/picker.go`
- 远程取值客户端在 `internal/geecache/grpc/getter.go`
- demo 启动入口在 `cmd/grpc-nodes/main.go`

## 1. etcd 是什么

`etcd` 是一个分布式、强一致的 KV 存储，常被用来做：

- 服务注册与发现
- 配置中心
- 分布式锁
- Leader 选举
- 保存集群元数据

它的核心价值不是“能存 key-value”，而是它同时提供了这几件事：

- 强一致读写
- 基于 lease 的临时节点
- watch 机制，可以持续订阅某个 key 或某个前缀下的变化
- 原子事务（Txn）

对于服务注册发现，`etcd` 最关键的能力是：

- 节点启动时写入自己的服务地址
- 这个地址绑定一个 `lease`
- 节点存活时持续续租
- 节点挂掉或网络断开足够久，`lease` 过期，注册信息自动删除
- 其他节点通过 `watch` 感知新增、删除事件，更新本地节点视图

这和当前项目的差异很直接：

- 现在：`cmd/grpc-nodes/main.go` 里把 `8001/8002/8003` 写死，然后 `srv.Set(addrs...)`
- 目标：节点地址不再写死，而是来自 `etcd` 中某个服务前缀下的动态列表

## 2. etcd 里和注册发现最相关的概念

### 2.1 Key / Value

最基础的数据结构就是 KV。

例如约定 geecache 的节点都注册在这个前缀下：

```text
/services/geecache/nodes/
```

某个节点可以写入：

```text
Key:   /services/geecache/nodes/node-8001
Value: {"addr":"localhost:8001","group":"scores"}
```

这里的 value 可以很简单，只存地址；也可以扩展为 JSON，附带权重、机房、版本等信息。

### 2.2 Lease

`lease` 可以理解成“带 TTL 的租约”。

当你执行“写入 key 并绑定 lease”后：

- 只要客户端持续续租，这个 key 就一直存在
- 一旦客户端退出或续租失败超过 TTL，这个 key 会被 etcd 自动删掉

对于服务发现，这是替代“手工下线节点”的关键机制。

### 2.3 KeepAlive

Go 客户端可以对 lease 调用 `KeepAlive`，后台会不断续租。

这意味着：

- 服务活着，注册信息就活着
- 服务死了，注册信息会自动消失

### 2.4 Watch

`Watch` 可以==订阅某个 key 或某个前缀的变化。==

对于服务发现，通常会 watch 一个前缀：

```text
/services/geecache/nodes/
```

这样任意节点：

- 新注册
- 下线
- 地址变更

都能实时推送到 watcher。

### 2.5 WithPrefix

这是 etcd Go SDK 里非常常用的选项。

- `Get(ctx, prefix, clientv3.WithPrefix())`：获取某个前缀下的所有 key
- `Watch(ctx, prefix, clientv3.WithPrefix())`：监听某个前缀下的所有变化

做服务发现时，几乎一定会用到。

## 3. etcd Go SDK 要掌握哪些 API

Go 里通常使用官方客户端：

```go
import clientv3 "go.etcd.io/etcd/client/v3"
```

你的 `go.mod` 之后会新增类似依赖：

```go
go.etcd.io/etcd/client/v3
```

下面只讲和这个项目最相关的 API。

### 3.1 创建客户端

```go
cli, err := clientv3.New(clientv3.Config{
    Endpoints:   []string{"127.0.0.1:2379"},
    DialTimeout: 5 * time.Second,
})
```

说明：

- `Endpoints` 是 etcd 集群地址，可以配置多个
- 客户端通常在进程生命周期内复用，不要每次操作都新建

### 3.2 Put / Get

```go
_, err = cli.Put(ctx, key, value)
resp, err := cli.Get(ctx, prefix, clientv3.WithPrefix())
```

说明：

- `Put` 用于注册
- `Get + WithPrefix` 用于服务启动时先全量拉取一次节点列表

### 3.3 Grant / KeepAlive

```go
leaseResp, err := cli.Grant(ctx, 10)
leaseID := leaseResp.ID

_, err = cli.Put(ctx, key, value, clientv3.WithLease(leaseID))
ch, err := cli.KeepAlive(ctx, leaseID)
```

说明：

- `Grant(ctx, 10)` 表示申请一个 TTL 为 10 秒的 lease
- `Put(..., WithLease(leaseID))` 表示把注册信息绑定到这个 lease
- `KeepAlive` 返回一个 channel，需要持续消费，确保续租链路正常

实践上通常会起一个 goroutine 持续读取：

```go
go func() {
    for resp := range ch {
        if resp == nil {
            return
        }
    }
}()
```

### 3.4 Watch

```go
watchCh := cli.Watch(ctx, prefix, clientv3.WithPrefix())
for watchResp := range watchCh {
    for _, ev := range watchResp.Events {
        switch ev.Type {
        case mvccpb.PUT:
            // 新节点或节点信息更新
        case mvccpb.DELETE:
            // 节点下线
        }
    }
}
```

说明：

- watch 只负责“增量变化”
- 一般需要先 `Get` 做一次全量同步，再 `Watch` 做增量更新

### 3.5 Revoke

优雅退出时可以主动撤销 lease：

```go
_, _ = cli.Revoke(ctx, leaseID)
```

即使你不主动调用，只要进程退出并且 lease 最终过期，key 也会被删；但优雅退出时主动 `Revoke` 更干净，节点摘除更快。

## 4. etcd 如何映射到 geecache 当前架构

### 4.1 当前架构的问题

现在节点成员关系由这段静态代码决定：

```go
addrMap := map[int]string{
    8001: "localhost:8001",
    8002: "localhost:8002",
    8003: "localhost:8003",
}

addrs := make([]string, 0, len(addrMap))
for _, v := range addrMap {
    addrs = append(addrs, v)
}

srv := peer.NewRPCPicker(addr)
srv.Set(addrs...)
```

这带来几个问题：

- 新节点加入，必须修改配置并重启
- 节点下线，一致性哈希环里仍然可能保留旧节点
- 没有统一的集群成员视图
- demo 可以运行，但不具备动态伸缩能力

### 4.2 改造后的职责划分

引入 etcd 后，可以把职责拆成两部分：

1. 注册端：当前节点把自己的 gRPC 地址注册到 etcd
2. 发现端：当前节点 watch etcd，动态刷新 `RPCPicker` 的节点列表

这件事对你现有代码的影响主要集中在 `grpc` 这一层，`Group` 本身不需要大改。

### 4.3 你真正要替换的点

本质上，只需要把下面这件事替换掉：

- 以前：手工调用一次 `RPCPicker.Set(peers...)`
- 以后：由 etcd discovery 组件在启动时全量同步一次，再在后台 watch 并反复调用更新逻辑

所以改造的核心不是缓存逻辑，而是“如何维护 `RPCPicker` 的成员列表”。

## 5. 推荐的集成方案

推荐增加一个独立的 etcd 服务发现模块，例如：

```text
internal/registry/etcd/
```

可以拆成两个角色：

- `Registrar`：负责把当前节点注册到 etcd，并维持 lease
- `Discovery`：负责订阅节点变化，并把最新地址列表推给 `RPCPicker`

### 5.1 etcd 键空间设计

建议统一前缀：

```text
/services/geecache/nodes/
```

每个节点一个 key：

```text
/services/geecache/nodes/{nodeID}
```

其中：

- `nodeID` 建议稳定且唯一，例如 `localhost:8001`
- value 建议至少包含 `addr`

例如：

```json
{
  "addr": "localhost:8001"
}
```

如果后续你要扩展，可以改成：

```json
{
  "id": "localhost:8001",
  "addr": "localhost:8001",
  "version": "v1",
  "weight": 1
}
```

对当前项目而言，最小可行版本其实只需要 `addr`。

### 5.2 RPCPicker 需要支持“全量替换”

你现在的 `RPCPicker.Set(peers ...string)` 更像初始化接口。

如果接入 etcd，建议把它语义上改成“用最新节点列表重建哈希环”，例如：

```go
func (r *RPCPicker) UpdatePeers(peers []string)
```

实现上要点是：

- 持锁
- 用新的节点列表重建一致性哈希 `Map`
- 重建 `getters`
- 保留自身节点 `r.Server.self`

为什么建议“全量替换”而不是“增删改事件逐条打补丁”：

- 逻辑更简单
- 不容易把哈希环状态修坏
- 节点规模一般不大，这里的重建成本很低
- watch 收到事件后，也可以先更新内存中的成员集合，再整体刷新 `RPCPicker`

### 5.3 服务启动流程

建议把 `cmd/grpc-nodes/main.go` 的启动流程改成：

1. 创建 `Group`
2. 创建 `RPCPicker`
3. 创建 etcd client
4. 启动 registrar，把当前节点注册到 etcd
5. 启动 discovery，先全量拉取节点，再 watch
6. discovery 每次拿到最新节点列表，就调用 `picker.UpdatePeers(...)`
7. `gee.RegisterPicker(picker)`
8. 启动 gRPC server
9. 收到退出信号时，停止 watch，撤销 lease，优雅退出 gRPC

注意顺序：

- `RegisterPicker` 只应调用一次，这和 `Group.RegisterPicker` 的实现一致
- 之后动态变化只更新 picker 内部状态，不要重复注册 picker

## 6. 建议新增的代码结构

可以按下面方式组织：

```text
internal/registry/etcd/client.go
internal/registry/etcd/registrar.go
internal/registry/etcd/discovery.go
internal/registry/etcd/types.go
```

职责建议如下。

### 6.1 `types.go`

定义节点元数据：

```go
type Node struct {
    ID   string `json:"id"`
    Addr string `json:"addr"`
}
```

### 6.2 `client.go`

负责构造 etcd client：

```go
func NewClient(endpoints []string) (*clientv3.Client, error)
```

这样 `cmd/grpc-nodes/main.go` 不必直接处理太多 etcd 细节。

### 6.3 `registrar.go`

对外提供类似接口：

```go
type Registrar struct {
    cli     *clientv3.Client
    prefix  string
    node    Node
    leaseID clientv3.LeaseID
}

func (r *Registrar) Register(ctx context.Context, ttl int64) error
func (r *Registrar) Deregister(ctx context.Context) error
```

注册逻辑：

1. `Grant` 一个 lease
2. `Put(prefix + node.ID, json(node), WithLease(leaseID))`
3. `KeepAlive`
4. 后台消费 keepalive channel

### 6.4 `discovery.go`

对外提供类似接口：

```go
type Discovery struct {
    cli    *clientv3.Client
    prefix string
}

func (d *Discovery) List(ctx context.Context) ([]Node, error)
func (d *Discovery) Watch(ctx context.Context, onUpdate func([]Node)) error
```

推荐实现方式：

- `List`：启动时先 `Get(prefix, WithPrefix())`
- `Watch`：后台 watch 前缀
- 内部维护一个 `map[string]Node`
- 每次收到 `PUT/DELETE` 事件，更新 map
- 把 map 转为 `[]string{addr1, addr2, ...}`
- 调用 `picker.UpdatePeers(...)`

## 7. 关键实现细节

### 7.1 为什么要“先 List，再 Watch”

如果你只 watch，不做启动时的全量拉取，会有一个明显问题：

- 当前节点启动时，etcd 里已经有若干已有节点
- 但这些历史状态不会自动作为 watch 事件重放给你
- 结果你的 picker 初始视图是不完整的

标准做法是：

1. `Get(prefix, WithPrefix())` 做全量初始化
2. `Watch(prefix, WithPrefix())` 订阅增量变化

### 7.2 `RPCPicker` 的并发安全

你现有的 `RPCPicker` 已经有 `mu sync.RWMutex`，这是对的。

引入动态更新后，并发模型会变成：

- 业务读路径：`Group.Get()` -> `PickPeer()`
- 后台写路径：`Discovery.Watch()` -> `UpdatePeers()`

因此要确保：

- `PickPeer()` 持读锁
- `UpdatePeers()` 持写锁
- 不要在无锁状态下修改 `peers` 和 `getters`

### 7.3 是否把自己加入哈希环

建议：加入。

原因是你当前 `PickPeer` 的逻辑已经处理了“选到自己时不走远程”的分支：

```go
if peer := r.peers.Get(key); peer != "" && peer != r.Server.self {
    return r.getters[peer], true
}
return nil, false
```

也就是说：

- 一致性哈希环里包含自己和其他节点
- 如果 key 命中自己，直接回退本地加载
- 如果 key 命中别人，才走远程

这个设计可以保留，不需要为了接入 etcd 而改语义。

### 7.4 `getter` 连接的生命周期

当前 `newPRCGetter` 会为每个地址创建一个 gRPC client。

动态发现之后，需要注意两个现实问题：

- 新节点加入时，要能创建新的 getter
- 节点删除时，旧 getter 最好能关闭底层连接，避免连接泄漏

因此更完整的方案是让 `rpcGetter` 持有 `*grpc.ClientConn`，并提供 `Close()`，例如：

```go
type rpcGetter struct {
    addr string
    conn *grpc.ClientConn
    client pb.GroupCacheClient
}
```

然后在 `UpdatePeers()` 里：

- 对新节点创建连接
- 对已删除节点关闭连接

这不是“接入 etcd 的第一步必须做完”的内容，但它是接入动态发现后应该尽快补上的工程项。

### 7.5 退出时的处理

demo 目前已经有信号处理，这很好。

接入 etcd 后，退出流程建议变成：

1. cancel discovery/watch 的 context
2. 主动 `Revoke` 当前节点 lease
3. `GracefulStop()` gRPC server
4. 关闭 etcd client

这样其他节点几乎能立刻感知当前节点下线，不必等 TTL 自然过期。

## 8. 对当前仓库的具体改造建议

下面按文件列出更贴近你仓库的改法。

### 8.1 `internal/geecache/grpc/picker.go`

当前问题：

- 只有 `Set(peers ...string)`，没有明确表达“动态刷新”的语义
- `getters` 初始化依赖 `Set`
- 没有清理旧 getter 的机制

建议改造：

- 保留 `PickPeer`
- 把 `Set` 改造成 `UpdatePeers`，或者新增 `UpdatePeers`
- `UpdatePeers` 内部全量重建哈希环
- 如果你准备顺手完善资源管理，就顺便关闭被移除节点的连接

伪代码：

```go
func (r *RPCPicker) UpdatePeers(peers []string) {
    r.mu.Lock()
    defer r.mu.Unlock()

    nextHash := consistenthash.New(defaultReplicas, nil)
    nextHash.Add(peers...)

    nextGetters := make(map[string]geecache.PeerGetter, len(peers))
    for _, peer := range peers {
        if old, ok := r.getters[peer]; ok {
            nextGetters[peer] = old
            continue
        }
        nextGetters[peer] = newPRCGetter(peer)
    }

    // 如果 getter 支持 Close，这里关闭已下线节点

    r.peers = nextHash
    r.getters = nextGetters
}
```

### 8.2 `internal/geecache/grpc/getter.go`

建议改造：

- 让 `rpcGetter` 保存 `*grpc.ClientConn`
- 提供 `Close()`
- 修复 `newPRCGetter` 这个命名笔误，改成 `newRPCGetter`

这部分不是 etcd 特有逻辑，但动态上下线后它会变得更重要。

### 8.3 `internal/geecache/grpc/server.go`

这里通常不需要和 etcd 强耦合。

保持职责单一即可：

- `RPCServer` 只负责提供 gRPC 服务
- 注册发现逻辑放在 `internal/registry/etcd`

### 8.4 `cmd/grpc-nodes/main.go`

这里是主要改造点。

建议新增启动参数：

```text
-etcd=127.0.0.1:2379
-node-id=localhost:8001
-service-prefix=/services/geecache/nodes/
```

启动逻辑从：

- 构造静态 `addrMap`
- `srv.Set(addrs...)`

改成：

- 创建 etcd client
- 注册自己
- 启动 discovery
- discovery 回调里执行 `picker.UpdatePeers(...)`

这样 demo 以后可以：

- 直接启动任意个节点
- 节点地址由命令行参数决定
- 只要都注册到同一前缀下，就自动形成集群

### 8.5 `Makefile`

如果你要保留 demo 易用性，可以增加：

- 启动本地 etcd 的说明
- 或新增 `run-etcd-demo` 这类目标

例如文档里说明先执行：

```bash
docker run --rm -p 2379:2379 -p 2380:2380 \
  quay.io/coreos/etcd:v3.5.18 \
  /usr/local/bin/etcd \
  --advertise-client-urls=http://0.0.0.0:2379 \
  --listen-client-urls=http://0.0.0.0:2379 \
  --listen-peer-urls=http://0.0.0.0:2380
```

然后再启动 geecache 节点。

## 9. 一个可行的最小实现路径

建议按下面顺序做，不要一开始就把功能铺太大。

### 第一步：先把 etcd client 跑通

目标：

- 项目能连接本地 etcd
- 能把一个节点地址写到指定前缀下

你可以先单独写一个很薄的注册代码，不接 discovery。

### 第二步：给注册加 lease 和 keepalive

目标：

- 进程活着，key 存在
- 进程停掉，key 自动删除

做到这一步，注册端就基本成型了。

### 第三步：实现 discovery 的全量拉取

目标：

- 启动时从 etcd 拉到所有现存节点
- 用这个结果初始化 `RPCPicker`

这一步完成后，即使没有 watch，也已经比静态配置更灵活了。

### 第四步：实现 watch 动态更新

目标：

- 新节点上线，hash ring 自动更新
- 节点下线，hash ring 自动更新

做到这里，核心目标才算完成。

### 第五步：补连接清理和退出流程

目标：

- 被移除节点的 gRPC 连接被关闭
- 进程退出时主动撤销 lease
- watch goroutine 可控退出

这一步属于工程收尾，但建议做。

## 10. 伪代码级接入示例

下面用伪代码把主流程串起来。

### 10.1 注册

```go
cli := mustNewEtcdClient([]string{"127.0.0.1:2379"})
registrar := etcd.NewRegistrar(cli, "/services/geecache/nodes/", Node{
    ID:   addr,
    Addr: addr,
})
if err := registrar.Register(ctx, 10); err != nil {
    return err
}
defer registrar.Deregister(context.Background())
```

### 10.2 发现 + 更新 picker

```go
picker := peer.NewRPCPicker(addr)
gee.RegisterPicker(picker)

discovery := etcd.NewDiscovery(cli, "/services/geecache/nodes/")

nodes, err := discovery.List(ctx)
if err != nil {
    return err
}
picker.UpdatePeers(nodeAddrs(nodes))

go func() {
    _ = discovery.Watch(ctx, func(nodes []Node) {
        picker.UpdatePeers(nodeAddrs(nodes))
    })
}()
```

### 10.3 启动服务

```go
rpcSrv := picker.Server.StartServer()
```

这个顺序的含义是：

- 节点先注册到 etcd
- 再同步集群成员
- 然后开始对外提供 gRPC 服务

如果你希望更稳妥，也可以先启动 gRPC，再注册自己。两者都可以，但要保证“注册到 etcd 的地址一定已经可用”或者即将可用，否则其他节点可能很快把请求打到一个还没 ready 的节点上。

## 11. 一些工程上的取舍建议

### 11.1 要不要把发现结果按 group 维度区分

看你当前项目，不需要。

原因是：

- `Group` 是缓存命名空间
- 节点成员关系属于“集群层面的信息”
- 当前一个节点通常会承载多个 group

因此注册信息按“节点”维度维护即可，不必按 group 单独注册。

### 11.2 要不要让每个 `Group` 都有自己的 discovery

不建议。

更合理的是：

- 一个 geecache 进程维护一个节点发现组件
- 一个 `RPCPicker` 反映整个节点集合
- 多个 group 共享这个 picker 或共享同一套节点视图

### 11.3 要不要直接使用 etcd resolver + gRPC LB

对当前项目，不建议作为第一步。

原因是你的核心路由逻辑不是简单的随机/轮询，而是：

- 先由一致性哈希决定 key 属于哪个节点
- 再直连那个节点

所以 etcd 在这里解决的是“节点成员发现”，不是“替代一致性哈希选点”。

## 12. 风险和注意事项

### 12.1 watch 事件不等于最终真相

watch 是增量流，代码里最好维护一个本地成员表，然后每次基于该成员表生成最新 `peers`。不要把某次单一事件直接当成完整成员列表。

### 12.2 lease TTL 不要太短

TTL 太短会带来：

- 网络抖动时频繁摘除节点
- 集群视图频繁变化
- 一致性哈希环频繁重建

demo 阶段可以先从 `10s` 或 `15s` 开始。

### 12.3 节点地址要可达

注册到 etcd 里的地址必须是其他节点真正可访问的地址。

在本地 demo 中：

- `localhost:8001` 没问题

如果以后部署到多机环境：

- 不能再随便注册 `localhost`
- 应注册 pod IP、service DNS 或机器内网地址

### 12.4 哈希环抖动是正常现象

节点上线和下线时，一致性哈希会导致部分 key 归属变化，这是预期行为，不是 bug。

etcd 只负责让成员关系真实、及时地反映出来。

## 13. 推荐落地顺序总结

如果你准备真正开始改代码，我建议按这个顺序提交：

1. 新增 `internal/registry/etcd`，实现 `client + registrar`
2. 给 `cmd/grpc-nodes/main.go` 增加 etcd 启动参数，并完成注册
3. 改造 `RPCPicker`，支持 `UpdatePeers([]string)`
4. 实现 `discovery.List()`，先完成启动时全量同步
5. 实现 `discovery.Watch()`，接入动态更新
6. 改造 `rpcGetter`，补连接关闭和资源回收
7. 最后补 README 或 demo 脚本

这个顺序的好处是每一步都能独立验证，不会把问题混在一起。

## 14. 结论

对当前 geecache 项目而言，`etcd` 的职责应该非常明确：

- 不负责缓存数据
- 不负责替代一致性哈希
- 只负责维护“当前有哪些可用节点”这件事

落到代码上，关键改造点只有两个：

1. 用 `Registrar` 把当前节点地址通过 `lease + keepalive` 注册到 etcd
2. 用 `Discovery` 通过 `List + Watch` 动态更新 `RPCPicker` 的节点集合

这样你的缓存访问链路仍然保持不变：

- `Group.Get(key)`
- `PickPeer(key)` 用一致性哈希决定节点
- 如果命中远程节点，就用 gRPC 去拿值

变化的只是：

- `PickPeer` 背后的节点列表，不再来自静态配置，而是来自 etcd 的实时集群视图

如果后续你要继续推进到代码实现，最先值得改的是这两处：

- `internal/geecache/grpc/picker.go`：增加动态更新能力
- `cmd/grpc-nodes/main.go`：把静态地址列表改成 etcd 注册发现流程

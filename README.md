[![Build Status](https://travis-ci.org/kubernetes-incubator/descheduler.svg?branch=master)](https://travis-ci.org/kubernetes-incubator/descheduler)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-incubator/descheduler)](https://goreportcard.com/report/github.com/kubernetes-incubator/descheduler)

# kubernetes二次调度

原始说明详见原版链接：

该版本中有些需要进行个性化修改的点：

1，duplicates策略中，对于检测同一节点上是否部署了同一deployment的pod的规则，可以根据个人需要进行修改：FindDuplicatePods

2，LowNodeUtilization策略中，节点的排序方法：SortNodesByUsage，其规则是CPU，MEM，PODs三项百分比相加之和进行比较，可以根据个人需要进行修改。

3，LowNodeUtilization策略中，未加入对Pod标签的校验。

4，policy文件无法动态生成，可以根据自身需要增加controller模块控制policy中的阈值。


逻辑问题汇总：

1，在驱逐最大负载节点上的pod时，如向指定驱逐某个pod，必须考虑所取的最大负载节点上是否存在该pod。否则其永远不会驱逐pod


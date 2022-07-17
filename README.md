## Elastic Queue

<div align="left"><img src="./static/logo.png" style="width:300px;" /></div>

*Running Your AI in a Kubernetes-Native Way.*

**NOTE:** Elastic Queue is based on [kueue](https://github.com/kubernetes-sigs/kueue) with more job controllers and production support.

---

Elastic Queue is a Kubernetes-native AI Workload Queue, which can make AI run in Kubernetes quickly and smoothly. 

Elastic Queue supports queuing of jobs with different priorities, automatic start and stop, and fine-grained allocation and on-demand use of GPU resources through GPU sharing and hot swapping. Elastic Queue provides multi-tenancy and quota capabilities that allow administrators to allocate resources independently to different users. Elastic Queue also supports multiple clouds, allowing AI services to run on different clouds according to their needs. Elastic Queue supports the integration of mainstream AI computing frameworks, such as TensorFlow, Pytorch, MXNet KubeFlow, etc., and can seamlessly dock with customers' original AI computing platform.

- **Cloud Native Job Queue** submits and schedules AI tasks in Kubernetes-native mode, which is consistent with the original concept of Kubernetes, and reduces customer awareness and cost of use.
- **Intelligent Scheduling** schedules multi-priority job, supports automatic job start and stop, uses GPU sharing and swapping to improve customer GPU utilization.
- **Flexible Quota** makes multiple capacity policies to support quota allocation (max-min fairness / DRF, etc.) to maximize the efficiency of cluster resource allocation.
- **Multi-tenant Isolation** uses namespace to achieve multi-tenancy, independent quota and resource isolation between tenants, so that different customers can share Kubernetes clusters.
- **Multi-cloud Scaler** runs different queues to different public cloud providers, and optimizes the running cost of AI tasks using multi-cloud.
- **Multi-framework Integration** integrates the mainstream AI computing framework, simplifies the access process and reduces customer access costs

## Prerequisites

- Kubernetes v1.22+

## Installation

1. Install Elastic Queue

```
$ kubectl apply -f https://github.com/elastic-ai/elastic-queue/releases/download/v0.1.0/manifests.yaml
```

2. Install Kubeflow training operator that supports Elastic Queue

```
$ kubectl apply -f https://github.com/elastic-ai/elastic-queue/releases/download/v0.1.0/kubeflow-training-operator-manifests.yaml
```

**Note:** Elastic Queue need job suspend which Kubeflow training operator do not yet supports. So we fork and release a supported version and has submitted the [PR](https://github.com/kubeflow/common/pull/196) to Kubeflow community. We will use community version after the PR is merged.

3. The controller runs in elastic-queue namespace

```
$ kubectl get pod -nelastic-queue
NAME                                                READY   STATUS    RESTARTS   AGE
elastic-queue-controller-manager-58496c48b7-xrsfv   1/1     Running   0          173m
```

## Getting Started

A simple example can be found in [samples](https://github.com/elastic-ai/elastic-queue/tree/master/config/samples). You can setup a ClusterQueue / Queue with:

```
$ kubectl apply -f config/samples/single-clusterqueue-setup.yaml
```

Later, you can run a tfjob with:

```
$ kubectl apply -f config/samples/sample-tfjob.yaml
```

Because Elastic Queue is based on kueue, so you also can run more samples in [kueue samples](https://github.com/kubernetes-sigs/kueue/blob/main/config/samples).

## Support / Contact

If you've got any questions, please feel free to contact us with following ways:
- [open a github issue](https://github.com/elastic-ai/elastic-queue/issues/new)
- [mailing list](mailto:elasticai@googlegroups.com) 
- [join discussion group](https://groups.google.com/g/elasticai)
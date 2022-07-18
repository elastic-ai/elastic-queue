## Elastic Queue

<div align="left"><img src="./static/logo.png" style="width:300px;" /></div>

*Running Your AI in a Kubernetes-Native Way.*

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

## Arena Support

Arena is a command-line interface to submit and manage kubeflow jobs. Now you can use it to submit tfjob with Elastic Queue support.

```
$ arena submit tfjob --name=tfjob-simple --image="knabben/tf-mnist-with-summaries:1.0" --queue="main" "'python /var/tf_mnist/mnist_with_summaries.py'"
$ arena list
NAME            STATUS     TRAINER  DURATION  GPU(Requested)  GPU(Allocated)  QUEUE  NODE
tfjob-simple    SUCCEEDED  TFJOB    57s       0               N/A             main   N/A
$ arena 
Name:      tfjob-simple
Status:    SUCCEEDED
Namespace: default
Queue:     main
Priority:  N/A
Trainer:   TFJOB
Duration:  57s

Instances:
  NAME                    STATUS     AGE  IS_CHIEF  GPU(Requested)  QUEUE  NODE
  ----                    ------     ---  --------  --------------  ----
  tfjob-simple-chief-0    Completed  57s  false     0               main  172.16.0.136
```

### Installation

You can find the supported arena release in [arena-queue](https://github.com/elastic-ai/arena/releases/download/v0.9.0-queue/arena-v0.9.0-queue.tgz).

```
$ wget https://github.com/elastic-ai/arena/releases/download/v0.9.0-queue/arena-v0.9.0-queue.tgz
$ tar zxvf arena-v0.9.0-queue.tgz -C /usr/bin
$ arena list
```

## Roadmap

- Support all Kubeflow training operator jobs (MPIJob, PytorchJob, XGBoostJob, MXJob)
- Support job resources can scale to cloud
- Integrate with Elastic GPU to support that the job can use gpushare resource

## Support / Contact

If you've got any questions, please feel free to contact us with following ways:
- [open a github issue](https://github.com/elastic-ai/elastic-queue/issues/new)
- [mailing list](mailto:elasticai@googlegroups.com) 
- [join discussion group](https://groups.google.com/g/elasticai)
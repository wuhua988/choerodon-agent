package deployment

import (
	"encoding/json"
	"fmt"
	"github.com/choerodon/choerodon-cluster-agent/manager"
	"time"

	"github.com/golang/glog"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appv1 "k8s.io/client-go/informers/extensions/v1beta1"
	appv1_lister "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/choerodon/choerodon-cluster-agent/pkg/model"
)

var (
	keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

type controller struct {
	queue workqueue.RateLimitingInterface
	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod  time.Duration
	lister            appv1_lister.DeploymentLister
	responseChan      chan<- *model.Packet
	deploymentsSynced cache.InformerSynced
	namespaces        *manager.Namespaces
}

func NewDeploymentController(deploymentInformer appv1.DeploymentInformer, responseChan chan<- *model.Packet, namespaces *manager.Namespaces) *controller {

	c := &controller{
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "deployment"),
		workerLoopPeriod: time.Second,
		lister:           deploymentInformer.Lister(),
		responseChan:     responseChan,
		namespaces:       namespaces,
	}

	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueueDeployment,
		UpdateFunc: func(old, new interface{}) {
			newDeployment := new.(*extensions.Deployment)
			oldDeployment := old.(*extensions.Deployment)
			if newDeployment.ResourceVersion == oldDeployment.ResourceVersion {
				return
			}
			c.enqueueDeployment(new)
		},
		DeleteFunc: c.enqueueDeployment,
	})
	c.deploymentsSynced = deploymentInformer.Informer().HasSynced
	return c
}

func (c *controller) Run(workers int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	if ok := cache.WaitForCacheSync(stopCh, c.deploymentsSynced); !ok {
		glog.Fatal("failed to wait for caches to sync")
	}

	//resources, err := c.lister.Deployments(c.namespace).List(labels.NewSelector())
	//if err != nil {
	//	glog.Fatal("failed list deployment")
	//} else {
	//	var resourceList []string
	//	for _, resource := range resources {
	//		if resource.Labels[model.ReleaseLabel] != "" {
	//			resourceList = append(resourceList, resource.GetName())
	//		}
	//	}
	//	resourceListResp := &kubernetes.ResourceList{
	//		Resources:    resourceList,
	//		ResourceType: "Deployment",
	//	}
	//	content, err := json.Marshal(resourceListResp)
	//	if err != nil {
	//		glog.Fatal("marshal deployment list error")
	//	} else {
	//		response := &model.Packet{
	//			Key:     fmt.Sprintf("env:%s", c.namespace),
	//			Type:    model.ResourceSync,
	//			Payload: string(content),
	//		}
	//		c.responseChan <- response
	//	}
	//}

	// Launch two workers to process Foo resources
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	glog.Info("Shutting down deployment workers")
}
func (c *controller) enqueueDeployment(obj interface{}) {
	var key string
	var err error
	if key, err = keyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.queue.AddRateLimited(key)
}

func (c *controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *controller) processNextWorkItem() bool {
	key, shutdown := c.queue.Get()

	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	forget, err := c.syncHandler(key.(string))
	if err == nil {
		if forget {
			c.queue.Forget(key)
		}
		return true
	}

	runtime.HandleError(fmt.Errorf("error syncing '%s': %s", key, err.Error()))
	c.queue.AddRateLimited(key)

	return true
}

func (c *controller) syncHandler(key string) (bool, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return true, nil
	}

	if !c.namespaces.Contain(namespace) {
		return true, nil
	}

	deployment, err := c.lister.Deployments(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			c.responseChan <- newDeploymentDelRep(name, namespace)
			glog.Warningf("deployment '%s' in work queue no longer exists", key)
			return true, nil
		}
		return false, err
	}

	if deployment.Labels[model.ReleaseLabel] != "" {
		glog.V(2).Info(deployment.Labels[model.ReleaseLabel], ":", deployment)
		c.responseChan <- newDeploymentRep(deployment)
	}
	return true, nil
}

func newDeploymentDelRep(name string, namespace string) *model.Packet {

	return &model.Packet{
		Key:  fmt.Sprintf("env:%s.Deployment:%s", namespace, name),
		Type: model.ResourceDelete,
	}
}

func newDeploymentRep(deployment *extensions.Deployment) *model.Packet {
	payload, err := json.Marshal(deployment)
	release := deployment.Labels[model.ReleaseLabel]
	if err != nil {
		glog.Error(err)
	}
	return &model.Packet{
		Key:     fmt.Sprintf("env:%s.release:%s.Deployment:%s", deployment.Namespace, release, deployment.Name),
		Type:    model.ResourceUpdate,
		Payload: string(payload),
	}
}

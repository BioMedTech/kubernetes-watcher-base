package watcher

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"time"
)

type ControllerInterface interface {
	ProcessItem(c *Controller, change *Change) []error
}

type Controller struct {
	ci       ControllerInterface
	Indexer  cache.Indexer
	Queue    workqueue.RateLimitingInterface
	informer cache.Controller
}

func NewController(queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		informer: informer,
		Indexer:  indexer,
		Queue:    queue,
	}
}

func (c *Controller) SetControllerInterface(ci ControllerInterface) {
	c.ci = ci
}

func (c *Controller) handleError(err []error, change *Change) {
	if err == nil || len(err) == 0 {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.Queue.Forget(change)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.Queue.NumRequeues(change) < 5 {
		klog.Infof("Error syncing virtualService %v: %v", change.Key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.Queue.AddRateLimited(change)
		return
	}

	c.Queue.Forget(change)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	for _, e := range err {
		runtime.HandleError(e)
	}
	klog.Infof("Dropping item %q out of the queue: %v", change.Key, err)
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	change, quit := c.Queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.Queue.Done(change)

	// Invoke the method containing the business logic
	err := c.ci.ProcessItem(c, change.(*Change))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleError(err, change.(*Change))
	return true
}

func (c *Controller) Run(threadiness int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.Queue.ShutDown()
	klog.Info("Starting controller")

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	klog.Info("Stopping Gateway controller")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

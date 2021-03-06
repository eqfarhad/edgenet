/*
Copyright 2020 Sorbonne Université

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authority

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"edgenet/pkg/authorization"
	appsinformer_v1 "edgenet/pkg/client/informers/externalversions/apps/v1alpha"

	log "github.com/Sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// The main structure of controller
type controller struct {
	logger   *log.Entry
	queue    workqueue.RateLimitingInterface
	informer cache.SharedIndexInformer
	handler  HandlerInterface
}

// The main structure of informerEvent
type informerevent struct {
	key      string
	function string
}

// Constant variables for events
const create = "create"
const update = "update"
const delete = "delete"
const failure = "Failure"
const success = "Successful"
const established = "Established"

// Start function is entry point of the controller
func Start() {
	clientset, err := authorization.CreateClientSet()
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}
	edgenetClientset, err := authorization.CreateEdgeNetClientSet()
	if err != nil {
		log.Println(err.Error())
		panic(err.Error())
	}

	authorityHandler := &Handler{}
	// Create the authority informer which was generated by the code generator to list and watch authority resources
	informer := appsinformer_v1.NewAuthorityInformer(
		edgenetClientset,
		0,
		cache.Indexers{},
	)
	// Create a work queue which contains a key of the resource to be handled by the handler
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var event informerevent
	// Event handlers deal with events of resources. Here, there are three types of events as Add, Update, and Delete
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// Put the resource object into a key
			event.key, err = cache.MetaNamespaceKeyFunc(obj)
			event.function = create
			log.Infof("Add authority: %s", event.key)
			if err == nil {
				// Add the key to the queue
				queue.Add(event)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			event.key, err = cache.MetaNamespaceKeyFunc(newObj)
			event.function = update
			log.Infof("Update authority: %s", event.key)
			if err == nil {
				queue.Add(event)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc helps to check the existence of the object while it is still contained in the index.
			// Put the resource object into a key
			event.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			event.function = delete
			log.Infof("Delete authority: %s", event.key)
			if err == nil {
				queue.Add(event)
			}
		},
	})
	controller := controller{
		logger:   log.NewEntry(log.New()),
		informer: informer,
		queue:    queue,
		handler:  authorityHandler,
	}

	// Cluster Roles for Authorities
	// Authority Admin
	policyRule := []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"users", "users/status", "userregistrationrequests",
		"userregistrationrequests/status", "slices", "slices/status", "teams", "teams/status", "nodecontributions"}, Verbs: []string{"*"}},
		{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"acceptableusepolicies"}, Verbs: []string{"get", "list"}}}
	authorityRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "authority-admin"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(authorityRole)
	if err != nil {
		log.Infof("Couldn't create authority-admin cluster role: %s", err)
		if errors.IsAlreadyExists(err) {
			authorityClusterRole, err := clientset.RbacV1().ClusterRoles().Get(authorityRole.GetName(), metav1.GetOptions{})
			if err == nil {
				authorityClusterRole.Rules = policyRule
				_, err = clientset.RbacV1().ClusterRoles().Update(authorityClusterRole)
				if err == nil {
					log.Infoln("Authority-admin cluster role updated")
				}
			}
		}
	}
	// Authority Manager
	policyRule = []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"userregistrationrequests", "userregistrationrequests/status",
		"slices", "slices/status", "teams", "teams/status"}, Verbs: []string{"*"}},
		{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"users", "acceptableusepolicies", "nodecontributions"}, Verbs: []string{"get", "list"}}}
	authorityRole = &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "authority-manager"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(authorityRole)
	if err != nil {
		log.Infof("Couldn't create authority-manager cluster role: %s", err)
		if errors.IsAlreadyExists(err) {
			authorityClusterRole, err := clientset.RbacV1().ClusterRoles().Get(authorityRole.GetName(), metav1.GetOptions{})
			if err == nil {
				authorityClusterRole.Rules = policyRule
				_, err = clientset.RbacV1().ClusterRoles().Update(authorityClusterRole)
				if err == nil {
					log.Infoln("Authority-manager cluster role updated")
				}
			}
		}
	}
	// Authority Tech
	policyRule = []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"nodecontributions"}, Verbs: []string{"*"}}}
	authorityRole = &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "authority-tech"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(authorityRole)
	if err != nil {
		log.Infof("Couldn't create authority-tech cluster role: %s", err)
		if errors.IsAlreadyExists(err) {
			authorityClusterRole, err := clientset.RbacV1().ClusterRoles().Get(authorityRole.GetName(), metav1.GetOptions{})
			if err == nil {
				authorityClusterRole.Rules = policyRule
				_, err = clientset.RbacV1().ClusterRoles().Update(authorityClusterRole)
				if err == nil {
					log.Infoln("Authority-tech cluster role updated")
				}
			}
		}
	}
	// Authority User
	policyRule = []rbacv1.PolicyRule{{APIGroups: []string{"apps.edgenet.io"}, Resources: []string{"slices", "teams", "nodecontributions"}, Verbs: []string{"get", "list"}}}
	authorityRole = &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "authority-user"},
		Rules: policyRule}
	_, err = clientset.RbacV1().ClusterRoles().Create(authorityRole)
	if err != nil {
		log.Infof("Couldn't create authority-user cluster role: %s", err)
		if errors.IsAlreadyExists(err) {
			authorityClusterRole, err := clientset.RbacV1().ClusterRoles().Get(authorityRole.GetName(), metav1.GetOptions{})
			if err == nil {
				authorityClusterRole.Rules = policyRule
				_, err = clientset.RbacV1().ClusterRoles().Update(authorityClusterRole)
				if err == nil {
					log.Infoln("Authority-user cluster role updated")
				}
			}
		}
	}

	// A channel to terminate elegantly
	stopCh := make(chan struct{})
	defer close(stopCh)
	// Run the controller loop as a background task to start processing resources
	go controller.run(stopCh)
	// A channel to observe OS signals for smooth shut down
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}

// Run starts the controller loop
func (c *controller) run(stopCh <-chan struct{}) {
	// A Go panic which includes logging and terminating
	defer utilruntime.HandleCrash()
	// Shutdown after all goroutines have done
	defer c.queue.ShutDown()
	c.logger.Info("run: initiating")
	c.handler.Init()
	// Run the informer to list and watch resources
	go c.informer.Run(stopCh)

	// Synchronization to settle resources one
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Error syncing cache"))
		return
	}
	c.logger.Info("run: cache sync complete")
	// Operate the runWorker
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

// To process new objects added to the queue
func (c *controller) runWorker() {
	log.Info("runWorker: starting")
	// Run processNextItem for all the changes
	for c.processNextItem() {
		log.Info("runWorker: processing next item")
	}

	log.Info("runWorker: completed")
}

// This function deals with the queue and sends each item in it to the specified handler to be processed.
func (c *controller) processNextItem() bool {
	log.Info("processNextItem: start")
	// Fetch the next item of the queue
	event, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(event)
	// Get the key string
	keyRaw := event.(informerevent).key
	// Use the string key to get the object from the indexer
	item, exists, err := c.informer.GetIndexer().GetByKey(keyRaw)
	if err != nil {
		if c.queue.NumRequeues(event.(informerevent).key) < 5 {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, retrying", event.(informerevent).key, err)
			c.queue.AddRateLimited(event.(informerevent).key)
		} else {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, no more retries", event.(informerevent).key, err)
			c.queue.Forget(event.(informerevent).key)
			utilruntime.HandleError(err)
		}
	}

	if !exists {
		if event.(informerevent).function == delete {
			c.logger.Infof("Controller.processNextItem: object deleted detected: %s", keyRaw)
			c.handler.ObjectDeleted(item)
		}
	} else {
		if event.(informerevent).function == create {
			c.logger.Infof("Controller.processNextItem: object created detected: %s", keyRaw)
			c.handler.ObjectCreated(item)
		} else if event.(informerevent).function == update {
			log.Println(event.(informerevent).key)
			c.logger.Infof("Controller.processNextItem: object updated detected: %s", keyRaw)
			c.handler.ObjectUpdated(item)
		}
	}
	c.queue.Forget(event.(informerevent).key)

	return true
}

/*
Copyright 2020 k0s authors

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
package server

import (
	"context"
	"fmt"
	"sync/atomic"

	config "github.com/k0sproject/k0s/pkg/apis/v1beta1"
	k0sv1beta1 "github.com/k0sproject/k0s/pkg/apis/v1beta1"
	"github.com/k0sproject/k0s/pkg/component"
	kubeutil "github.com/k0sproject/k0s/pkg/kubernetes"
	"github.com/k0sproject/k0s/pkg/leaderelection"
	"github.com/sirupsen/logrus"
)

// LeaderElector is the common leader elector component to manage each controller leader status
type LeaderElector interface {
	IsLeader() bool
	component.Component
}

type leaderElector struct {
	ClusterConfig *config.ClusterConfig

	L *logrus.Entry

	stopCh            chan struct{}
	leaderStatus      atomic.Value
	kubeClientFactory kubeutil.ClientFactory
	leaseCancel       context.CancelFunc
}

// NewLeaderElector creates new leader elector
func NewLeaderElector(c *k0sv1beta1.ClusterConfig, kubeClientFactory kubeutil.ClientFactory) LeaderElector {
	d := atomic.Value{}
	d.Store(false)
	return &leaderElector{
		ClusterConfig:     c,
		stopCh:            make(chan struct{}),
		kubeClientFactory: kubeClientFactory,
		L:                 logrus.WithFields(logrus.Fields{"component": "endpointreconciler"}),
		leaderStatus:      d,
	}
}

func (l *leaderElector) Init() error {
	return nil
}

func (l *leaderElector) Run() error {
	client, err := l.kubeClientFactory.Create()
	if err != nil {
		return fmt.Errorf("can't create kubernetes rest client for lease pool: %v", err)
	}
	leasePool, err := leaderelection.NewLeasePool(client, "k0s-endpoint-reconciler", leaderelection.WithLogger(l.L))

	if err != nil {
		return err
	}
	events, cancel, err := leasePool.Watch()
	if err != nil {
		return err
	}
	l.leaseCancel = cancel

	go func() {
		for {
			select {
			case <-events.AcquiredLease:
				l.L.Info("acquired leader lease")
				l.leaderStatus.Store(true)
			case <-events.LostLease:
				l.L.Info("lost leader lease")
				l.leaderStatus.Store(false)
			}
		}
	}()
	return nil
}

func (l *leaderElector) Stop() error {
	if l.leaseCancel != nil {
		l.leaseCancel()
	}
	return nil
}

func (l *leaderElector) IsLeader() bool {
	return l.leaderStatus.Load().(bool)
}

func (l *leaderElector) Healthy() error { return nil }

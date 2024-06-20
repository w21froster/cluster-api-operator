/*
Copyright 2023 The Kubernetes Authors.

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

package cmd

import (
	"context"
	"fmt"
	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"os"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	configclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

type moveOptions struct {
	fromKubeconfig        string
	fromKubeconfigContext string
	toKubeconfig          string
	toKubeconfigContext   string
	namespace             string
	fromDirectory         string
	toDirectory           string
	dryRun                bool
}

var moveOpts = &moveOptions{}

var moveCmd = &cobra.Command{
	Use:     "move",
	GroupID: groupManagement,
	Short:   "Move Cluster API objects and all dependencies between management clusters",
	Long: LongDesc(`
		Move Cluster API objects and all dependencies between management clusters.

		Note: The destination cluster MUST have the required provider components installed.`),

	Example: Examples(`
		Move Cluster API objects and all dependencies between management clusters.
		capioperator move --to-kubeconfig=target-kubeconfig.yaml

		Write Cluster API objects and all dependencies from a management cluster to directory.
		capioperator move --to-directory /tmp/backup-directory

		Read Cluster API objects and all dependencies from a directory into a management cluster.
		capioperator move --from-directory /tmp/backup-directory
	`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMove()
	},
}

func init() {
	moveCmd.Flags().StringVar(&moveOpts.fromKubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file for the source management cluster. If unspecified, default discovery rules apply.")
	moveCmd.Flags().StringVar(&moveOpts.toKubeconfig, "to-kubeconfig", "",
		"Path to the kubeconfig file to use for the destination management cluster.")
	moveCmd.Flags().StringVar(&moveOpts.fromKubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file for the source management cluster. If empty, current context will be used.")
	moveCmd.Flags().StringVar(&moveOpts.toKubeconfigContext, "to-kubeconfig-context", "",
		"Context to be used within the kubeconfig file for the destination management cluster. If empty, current context will be used.")
	moveCmd.Flags().StringVarP(&moveOpts.namespace, "namespace", "n", "",
		"The namespace where the workload cluster is hosted. If unspecified, the current context's namespace is used.")
	moveCmd.Flags().BoolVar(&moveOpts.dryRun, "dry-run", false,
		"Enable dry run, don't really perform the move actions")
	moveCmd.Flags().StringVar(&moveOpts.toDirectory, "to-directory", "",
		"Write Cluster API objects and all dependencies from a management cluster to directory.")
	moveCmd.Flags().StringVar(&moveOpts.fromDirectory, "from-directory", "",
		"Read Cluster API objects and all dependencies from a directory into a management cluster.")

	moveCmd.MarkFlagsMutuallyExclusive("to-directory", "to-kubeconfig")
	moveCmd.MarkFlagsMutuallyExclusive("from-directory", "to-directory")
	moveCmd.MarkFlagsMutuallyExclusive("from-directory", "kubeconfig")

	RootCmd.AddCommand(moveCmd)
}

func runMove() error {
	ctx := context.Background()

	if moveOpts.toDirectory == "" &&
		moveOpts.fromDirectory == "" &&
		moveOpts.toKubeconfig == "" &&
		!moveOpts.dryRun {
		return errors.New("please specify a target cluster using the --to-kubeconfig flag when not using --dry-run, --to-directory or --from-directory")
	}

	return moveProvider(ctx, moveOpts)
}

func moveProvider(ctx context.Context, opts *moveOptions) error {
	// Both backup and restore makes no sense. It's a complete move.
	if opts.fromDirectory != "" && opts.toDirectory != "" {
		return errors.Errorf("can't set both FromDirectory and ToDirectory")
	}

	if !opts.dryRun &&
		opts.fromDirectory == "" &&
		opts.toDirectory == "" &&
		opts.toKubeconfig == "" {
		return errors.Errorf("at least one of FromDirectory, ToDirectory and ToKubeconfig must be set")
	}

	if opts.toDirectory != "" {
		return toDirectory(ctx, opts)
	} else if opts.fromDirectory != "" {
		return fromDirectory(ctx, opts)
	}

	return moveProvider(ctx, opts)
}

func move(ctx context.Context, opts *moveOptions) error {
	// Get the client for interacting with the source management cluster.
	fromCluster, err := getClusterClient(ctx, cluster.Kubeconfig{Path: opts.fromKubeconfig, Context: opts.fromKubeconfigContext})
	if err != nil {
		return err
	}

	// If the option specifying the Namespace is empty, try to detect it.
	if opts.namespace == "" {
		currentNamespace, err := fromCluster.Proxy().CurrentNamespace()
		if err != nil {
			return err
		}
		opts.namespace = currentNamespace
	}

	var toCluster cluster.Client
	if !opts.dryRun {
		// Get the client for interacting with the target management cluster.
		if toCluster, err = getClusterClient(ctx, cluster.Kubeconfig{Path: opts.toKubeconfig, Context: opts.toKubeconfigContext}); err != nil {
			return err
		}
	}

	return fromCluster.ObjectMover().Move(ctx, opts.namespace, toCluster, opts.dryRun)
}

func fromDirectory(ctx context.Context, opts *moveOptions) error {
	toCluster, err := getClusterClient(ctx, cluster.Kubeconfig{Path: opts.toKubeconfig, Context: opts.toKubeconfigContext})
	if err != nil {
		return err
	}

	if _, err := os.Stat(opts.fromDirectory); os.IsNotExist(err) {
		return err
	}

	return toCluster.ObjectMover().FromDirectory(ctx, toCluster, opts.fromDirectory)
}

func toDirectory(ctx context.Context, opts *moveOptions) error {
	fromCluster, err := getClusterClient(ctx, cluster.Kubeconfig{Path: opts.fromKubeconfig, Context: opts.fromKubeconfigContext})
	if err != nil {
		return err
	}

	// If the option specifying the Namespace is empty, try to detect it.
	if opts.namespace == "" {
		currentNamespace, err := fromCluster.Proxy().CurrentNamespace()
		if err != nil {
			return err
		}
		opts.namespace = currentNamespace
	}

	if _, err := os.Stat(opts.toDirectory); os.IsNotExist(err) {
		return err
	}

	return fromCluster.ObjectMover().ToDirectory(ctx, opts.namespace, opts.toDirectory)
}

func getClusterClient(ctx context.Context, kubeconfig cluster.Kubeconfig) (cluster.Client, error) {
	configClient, err := configclient.New(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("cannot create config client: %w", err)
	}

	cluster := cluster.New(
		kubeconfig,
		configClient,
	)
	if err != nil {
		return nil, err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := cluster.ProviderInventory().CheckCAPIContract(ctx); err != nil {
		return nil, err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(ctx); err != nil {
		return nil, err
	}
	return cluster, nil
}

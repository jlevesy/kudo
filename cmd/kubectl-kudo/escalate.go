package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	kudoclientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
)

func newEscalateCmd() *cobra.Command {
	config := runEscalateCfg{
		ConfigFlags: genericclioptions.NewConfigFlags(true),
	}

	cmd := cobra.Command{
		Use:          "escalate",
		Short:        "Create a new kudo escalation",
		SilenceUsage: true,
		Long: `Kudo escalate creates a new kudo escalation.

Examples:
  To escalate using the policy "gain-read-configmaps" during the default duration on the default namespace, run:
    kubectl kudo escalate gain-read-configmaps --reason="Need access to configmaps"

  To escalate using the policy "gain-read-configmaps" during the default duration on the namespace application-a, run:
    kubectl kudo escalate gain-read-configmaps --namespace=appliation-a --reason="Need access to configmaps"

  To escalate using the policy "gain-read-configmaps" during 30s on the namespace application-a, run:
    kubectl kudo escalate gain-read-configmaps --namespace=appliation-a --duration=30s --reason="Need access to configmaps"

Find more information at:
	https://github.com/jlevesy/kudo
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEscalate(cmd, config, args)
		},
	}

	cmd.Flags().BoolVar(&config.noWait, "no-wait", false, "do not wait for escalation to be accepted, or denied")
	cmd.Flags().DurationVar(&config.duration, "duration", 0, "escalate for the given duration, defaults to the policy default duration")
	cmd.Flags().StringVar(&config.reason, "reason", "", "reason for the escalation (required)")
	config.ConfigFlags.AddFlags(cmd.Flags())

	return &cmd
}

type runEscalateCfg struct {
	*genericclioptions.ConfigFlags
	noWait   bool
	duration time.Duration
	reason   string
}

func runEscalate(cmd *cobra.Command, config runEscalateCfg, args []string) error {
	parsedArgs, err := parseEscalateArgs(args)
	if err != nil {
		return cmd.Help()
	}

	k8sConfig, err := config.ConfigFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	kudoClient, err := kudoclientset.NewForConfig(k8sConfig)
	if err != nil {
		return err
	}

	fmt.Println("Creating a new escalation request using policy", parsedArgs.policyName)

	escalation, err := kudoClient.K8sV1alpha1().Escalations().Create(
		cmd.Context(),
		&kudov1alpha1.Escalation{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kudo-escalation-",
			},
			Spec: kudov1alpha1.EscalationSpec{
				PolicyName: parsedArgs.policyName,
				Reason:     config.reason,
				Namespace:  *config.ConfigFlags.Namespace,
				Duration:   metav1.Duration{Duration: config.duration},
			},
		},
		metav1.CreateOptions{},
	)

	if err != nil {
		return fmt.Errorf("unable to create a new escalation, reason is: %w", err)
	}

	fmt.Println("Successfuly created escalation", escalation.Name)

	if config.noWait {
		return nil
	}

	fmt.Println("Waiting for the escalation", escalation.Name, "to be reviewed by kudo")

	watchHandler, err := kudoClient.K8sV1alpha1().Escalations().Watch(
		cmd.Context(),
		metav1.ListOptions{
			FieldSelector:   "metadata.name=" + escalation.Name,
			Watch:           true,
			ResourceVersion: escalation.ResourceVersion,
		},
	)

	if err != nil {
		return fmt.Errorf("unable to watch escalation, reason is: %w", err)
	}

	defer watchHandler.Stop()

	for {
		select {
		case <-cmd.Context().Done():
			return fmt.Errorf("watch context has expired: %w", cmd.Context().Err())
		case event, ok := <-watchHandler.ResultChan():
			if !ok {
				return errors.New("watch unexpectedly ended")
			}

			if event.Type != watch.Modified {
				return fmt.Errorf("received an unexpected event %q while watching", event.Type)
			}

			escalation, ok := event.Object.(*kudov1alpha1.Escalation)
			if !ok {
				return errors.New("received an object that is not an escalation")
			}

			switch escalation.Status.State {
			case kudov1alpha1.StatePending, kudov1alpha1.StateUnknown:
				// We're still pending, wait for another update.
				continue
			case kudov1alpha1.StateAccepted:
				// Escalation has been accepeted, success!
				fmt.Println("You have now augmented permissions, use it with care!")
				return nil
			case kudov1alpha1.StateDenied:
				return fmt.Errorf("Escalation has been denied, reason is: %s", escalation.Status.StateDetails)
			case kudov1alpha1.StateExpired:
				return fmt.Errorf("Escalation has expired, reason is: %s", escalation.Status.StateDetails)
			}
		}
	}
}

type escalateArgs struct {
	policyName string
}

func parseEscalateArgs(args []string) (escalateArgs, error) {
	if len(args) < 1 {
		return escalateArgs{}, errors.New("you need to provide a policy name")
	}

	parsedArgs := escalateArgs{
		policyName: args[0],
	}

	if strings.TrimSpace(parsedArgs.policyName) == "" {
		return escalateArgs{}, errors.New("you need to provide a non blank policy name")
	}

	return parsedArgs, nil
}

package main

import (
	"fmt"

	"github.com/pulumi/pulumi-linode/sdk/v4/go/linode"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		sshKey := cfg.RequireSecret("ssh-key")

		// Create a linode resource (Linode Instance)
		instance, err := linode.NewInstance(ctx, "my-linode", &linode.InstanceArgs{
			AuthorizedKeys: pulumi.StringArray{
				sshKey,
			},
			RootPass: cfg.RequireSecret("root"),
			Type:     pulumi.String("g6-nanode-1"),
			Region:   pulumi.String("eu-central"),
			Image:    pulumi.String("linode/debian11"),
		})
		if err != nil {
			return fmt.Errorf("instance: %w", err)
		}

		fmt.Println("creating new domain...")
		_, err = linode.NewDomain(ctx, "matus.se", &linode.DomainArgs{
			Domain:   pulumi.String("matus.se"),
			SoaEmail: pulumi.String("adrianforsius@gmail.com"),
			Type:     pulumi.String("master"),
		})
		if err != nil {
			return fmt.Errorf("domain: %w", err)
		}

		// Export the DNS name of the instance
		ctx.Export("instanceIpAddress", instance.IpAddress)
		return nil
	})
}

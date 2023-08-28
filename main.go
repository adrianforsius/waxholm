package main

import (
	"fmt"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
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

		instance.ID().ApplyT(func(id string) error {
			_, err := remote.NewCopyFile(ctx, "docker-compose-copy", &remote.CopyFileArgs{
				Connection: remote.ConnectionArgs{Host: pulumi.String(id)},
				LocalPath:  pulumi.String("docker-compose.yml"),
				RemotePath: pulumi.String("~/"),
			})
			if err != nil {
				return fmt.Errorf("copy: %w", err)
			}

			_, err = remote.NewCommand(ctx, "install deps", &remote.CommandArgs{
				Connection: remote.ConnectionArgs{Host: pulumi.String(id)},
				// curl is already installed in the instance image
				Create: pulumi.String("curl -L https://github.com/docker/compose/releases/download/1.25.3/docker-compose-`uname -s`-`uname -m` -o /usr/local/bin/docker-compose && sudo chmod +x /usr/local/bin/docker-compose"),
			})
			if err != nil {
				return fmt.Errorf("deps: %w", err)
			}

			return nil
		})

		// Export the DNS name of the instance
		ctx.Export("instanceIpAddress", instance.IpAddress)
		return nil
	})
}

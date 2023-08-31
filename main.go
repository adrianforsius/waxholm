package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi-linode/sdk/v4/go/linode"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		publicKeyPath := cfg.Get("ssh_public_key_path")
		publicKey, err := os.ReadFile(publicKeyPath)
		if err != nil {
			return fmt.Errorf("read ssh file: %w", err)
		}
		fmt.Println("public key", string(publicKey))

		privateKeyPath := cfg.Get("ssh_private_key_path")
		ctx.Log.Info(fmt.Sprintf("public key path: %s", privateKeyPath), nil)
		privateKey, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return fmt.Errorf("read ssh file: %w", err)
		}

		// Create a linode resource (Linode Instance)
		ctx.Log.Info("creating new instance...", nil)
		instance, err := linode.NewInstance(ctx, "my-linode", &linode.InstanceArgs{
			AuthorizedKeys: pulumi.StringArray{
				pulumi.String(strings.TrimSpace(string(publicKey))),
			},
			RootPass: cfg.RequireSecret("root"),
			Type:     pulumi.String("g6-nanode-1"),
			Region:   pulumi.String("eu-central"),
			Image:    pulumi.String("linode/debian11"),
		})
		if err != nil {
			return fmt.Errorf("instance: %w", err)
		}

		ctx.Log.Info("creating new domain...", nil)
		_, err = linode.NewDomain(ctx, "matus.se", &linode.DomainArgs{
			Domain:   pulumi.String("matus.se"),
			SoaEmail: pulumi.String("adrianforsius@gmail.com"),
			Type:     pulumi.String("master"),
		})
		if err != nil {
			return fmt.Errorf("domain: %w", err)
		}

		instance.IpAddress.ApplyT(func(ip string) error {
			ctx.Log.Info(fmt.Sprintf("copying files to ip: %s", ip), nil)
			_, err := remote.NewCopyFile(ctx, "docker-compose-copy", &remote.CopyFileArgs{
				Connection: remote.ConnectionArgs{
					Host:               pulumi.String(ip),
					Password:           cfg.RequireSecret("root"),
					PrivateKey:         pulumi.String(privateKey),
					PrivateKeyPassword: cfg.RequireSecret("ssh_private_key_pass"),
				},
				LocalPath:  pulumi.String("docker-compose.yml"),
				RemotePath: pulumi.String("/root/docker-compose.yml"),
			})
			if err != nil {
				return fmt.Errorf("copy: %w", err)
			}

			ctx.Log.Info(fmt.Sprintf("install deps using ip: %s", ip), nil)
			cmd, err := remote.NewCommand(ctx, "install-deps", &remote.CommandArgs{
				Connection: remote.ConnectionArgs{
					Host:               pulumi.String(ip),
					Password:           cfg.RequireSecret("root"),
					PrivateKey:         pulumi.String(privateKey),
					PrivateKeyPassword: cfg.RequireSecret("ssh_private_key_pass"),
				},
				// curl is already installed in the instance image
				Create: pulumi.String("curl -L https://github.com/docker/compose/releases/download/1.25.3/docker-compose-`uname -s`-`uname -m` -o /usr/local/bin/docker-compose && su - && chmod +x /usr/local/bin/docker-compose"),
			})
			if err != nil {
				return fmt.Errorf("deps: %w", err)
			}

			ctx.Export("deps-out", cmd.Stdout)

			return nil
		})

		// Export the DNS name of the instance
		ctx.Export("instanceIpAddress", instance.IpAddress)
		return nil
	})
}

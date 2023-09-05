package main

import (
	"fmt"
	"os"
	"strconv"
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
		domain, err := linode.NewDomain(ctx, "adrianforsiusconsulting.se", &linode.DomainArgs{
			Domain:   pulumi.String("adrianforsiusconsulting.se"),
			SoaEmail: pulumi.String("adrianforsius@gmail.com"),
			Type:     pulumi.String("master"),
		})
		if err != nil {
			return fmt.Errorf("domain: %w", err)
		}
		var domainID int
		domain.ID().ApplyT(func(id string) error {
			domainID, err = strconv.Atoi(id)
			if err != nil {
				return fmt.Errorf("invalid id: %w", err)
			}
			return nil
		})

		instance.IpAddress.ApplyT(func(ip string) error {
			_, err = linode.NewDomainRecord(ctx, "A", &linode.DomainRecordArgs{
				DomainId:   pulumi.Int(domainID),
				RecordType: pulumi.String("A"),
				Target:     pulumi.String(ip),
			})
			if err != nil {
				return fmt.Errorf("record A: %w", err)
			}

			_, err = linode.NewDomainRecord(ctx, "cloud", &linode.DomainRecordArgs{
				DomainId:   pulumi.Int(domainID),
				Name:       pulumi.String("cloud"),
				RecordType: pulumi.String("CNAME"),
				Target:     pulumi.String("adrianforsiusconsulting.se"),
			})
			if err != nil {
				return fmt.Errorf("cloud record: %w", err)
			}

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
				Create: pulumi.String("sudo apt-get update && sudo apt-get install -y ca-certificates curl gnupg && sudo install -m 0755 -d /etc/apt/keyrings && curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg && sudo chmod a+r /etc/apt/keyrings/docker.gpg && echo \"deb [arch=\"$(dpkg --print-architecture)\" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \"$(. /etc/os-release && echo \"$VERSION_CODENAME\")\" stable\" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null && sudo apt-get update && sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"),
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

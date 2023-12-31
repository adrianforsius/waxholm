package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"

	// "github.com/pulumi/pulumi-command/sdk/go/command/remote"
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

			domainName := "adrianforsiusconsulting.se"
			ctx.Log.Info("creating new domain...", nil)
			domain, err := linode.NewDomain(ctx, domainName, &linode.DomainArgs{
				Domain:   pulumi.String(domainName),
				SoaEmail: pulumi.String("adrianforsius@gmail.com"),
				Type:     pulumi.String("master"),
			})
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			domain.ID().ApplyT(func(id string) error {
				domainID, err := strconv.Atoi(id)
				if err != nil {
					return fmt.Errorf("invalid id: %w", err)
				}

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
					Target:     pulumi.String(domainName),
				})
				if err != nil {
					return fmt.Errorf("cloud record: %w", err)
				}

				_, err = linode.NewDomainRecord(ctx, "pihole", &linode.DomainRecordArgs{
					DomainId:   pulumi.Int(domainID),
					Name:       pulumi.String("pihole"),
					RecordType: pulumi.String("CNAME"),
					Target:     pulumi.String(domainName),
				})
				if err != nil {
					return fmt.Errorf("pihole record: %w", err)
				}

				_, err = linode.NewDomainRecord(ctx, "traefik", &linode.DomainRecordArgs{
					DomainId:   pulumi.Int(domainID),
					Name:       pulumi.String("traefik"),
					RecordType: pulumi.String("CNAME"),
					Target:     pulumi.String(domainName),
				})
				if err != nil {
					return fmt.Errorf("traefik record: %w", err)
				}

				_, err = linode.NewDomainRecord(ctx, "www", &linode.DomainRecordArgs{
					DomainId:   pulumi.Int(domainID),
					Name:       pulumi.String("www"),
					RecordType: pulumi.String("CNAME"),
					Target:     pulumi.String(domainName),
				})
				if err != nil {
					return fmt.Errorf("www record: %w", err)
				}

				return nil
			})

			ctx.Log.Info(fmt.Sprintf("install deps using ip: %s", ip), nil)
			pythonCmd, err := remote.NewCommand(ctx, "ansibleReqs", &remote.CommandArgs{
				Connection: &remote.ConnectionArgs{
					Host:               pulumi.String(ip),
					Port:               pulumi.Float64(22),
					PrivateKey:         pulumi.String(privateKey),
					PrivateKeyPassword: cfg.RequireSecret("ssh_private_key_pass"),
				},
				Create: pulumi.String("sudo apt-get update -y && sudo apt-get install python3 -y\n"),
			})
			if err != nil {
				return fmt.Errorf("ansible reqs: %w", err)
			}

			renderCmd, err := local.NewCommand(ctx, "playbookEnvs", &local.CommandArgs{
				Create: pulumi.String("cat playbook.yml | envsubst > playbook.with-envs.yml"),
				Environment: pulumi.StringMap{
					"PIHOLE_PASSWORD": cfg.RequireSecret("pihole_password"),
					"TS_AUTHKEY":      cfg.RequireSecret("tailscale_auth_key"),
					"LINODE_TOKEN":    cfg.RequireSecret("linode_token"),
				},
			})
			if err != nil {
				return fmt.Errorf("envs: %w", err)
			}

			cmd, err := local.NewCommand(ctx, "playbookRun", &local.CommandArgs{
				Create: pulumi.String(fmt.Sprintf("ANSIBLE_HOST_KEY_CHECKING=False ansible-playbook -i '%s,' playbook.with-envs.yml", ip)),
			}, pulumi.DependsOn([]pulumi.Resource{
				renderCmd,
				pythonCmd,
			}))
			if err != nil {
				return fmt.Errorf("playbook: %w", err)
			}

			ctx.Export("deps-out", cmd.Stdout)

			return nil
		})

		// Export the DNS name of the instance
		ctx.Export("instanceIpAddress", instance.IpAddress)
		return nil
	})
}

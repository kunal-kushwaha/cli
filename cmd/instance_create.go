package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"

	"github.com/civo/civogo"
	"github.com/civo/cli/config"
	"github.com/civo/cli/utility"

	"github.com/spf13/cobra"
)

var wait bool
var hostnameCreate, size, diskimage, publicip, initialuser, sshkey, tags, network, firewall string

var instanceCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new instance",
	Example: "civo instance create --hostname=foo.example.com [flags]",
	Long: `You can create an instance with a hostname parameter, as well as any options you provide.
If you wish to use a custom format, the available fields are:

	* id
	* hostname
	* size
	* region
	* public_ip
	* status
	* network_id
	* diskimage_id
	* initial_user
	* ssh_key
	* notes
	* firewall_id
	* tags
	* script
	* created_at
	* reverse_dns
	* private_ip
	* public_ip`,
	Run: func(cmd *cobra.Command, args []string) {
		utility.EnsureCurrentRegion()

		check, region, err := utility.CheckAvailability("iaas", regionSet)
		if err != nil {
			utility.Error("Error checking availability %s", err)
			os.Exit(1)
		}

		if check {
			utility.Error("Sorry you can't create a instance in the %s region", region)
			os.Exit(1)
		}

		client, err := config.CivoAPIClient()
		if regionSet != "" {
			client.Region = regionSet
		}
		if err != nil {
			utility.Error("Creating the connection to Civo's API failed with %s", err)
			os.Exit(1)
		}

		config, err := client.NewInstanceConfig()
		if err != nil {
			utility.Error("Unable to create a new config for the instance %s", err)
			os.Exit(1)
		}

		if hostnameCreate != "" {
			if utility.ValidNameLength(hostnameCreate) {
				utility.Warning("the hostname cannot be longer than 63 characters")
				os.Exit(1)
			}
			config.Hostname = hostnameCreate
		}

		if region != "" {
			config.Region = region
		}

		sizes, err := client.ListInstanceSizes()
		if err != nil {
			utility.Error("Unable to list instance sizes %s", err)
			os.Exit(1)
		}

		sizeIsValid := false
		for _, s := range sizes {
			if size == s.Name {
				sizeIsValid = true
				break
			}
		}

		if !sizeIsValid {
			utility.Error("The provided size is not valid")
			os.Exit(1)
		}
		config.Size = size

		diskImage, err := client.FindDiskImage(diskimage)
		if err != nil {
			utility.Error("DiskImage %s", err)
			os.Exit(1)
		}
		config.TemplateID = diskImage.ID


		if publicip != "" {
			config.PublicIPRequired = publicip
		}

		if initialuser != "" {
			config.InitialUser = initialuser
		}

		if sshkey != "" {
			sshKey, err := client.FindSSHKey(sshkey)
			if err != nil {
				utility.Error("SSHKey %s", err)
				os.Exit(1)
			}
			config.SSHKeyID = sshKey.ID
		}

		var net = &civogo.Network{}
		if network != "" {
			net, err = client.FindNetwork(network)
			if err != nil {
				utility.Error("Network %s", err)
				os.Exit(1)
			}
			config.NetworkID = net.ID

			if !net.Default && firewall == "" {
				utility.Error("Firewall is required when launching instance in non-default network. See '--firewall' flag.")
				os.Exit(1)
			}
		} else {
			net, err = client.GetDefaultNetwork()
			if err != nil {
				utility.Error("Unable to retrieve default network - %s", err)
				os.Exit(1)
			}
		}

		if firewall != "" {
			fw, err := client.FindFirewall(firewall)
			if err != nil {
				utility.Error("Unable to find firewall %s", err)
				os.Exit(1)
			}

			if net.ID != fw.NetworkID {
				utility.Error("%q firewall does not exist in %q network. Please try again.", firewall, net.Label)
				os.Exit(1)
			}

			config.FirewallID = fw.ID
		}

		if tags != "" {
			config.TagsList = tags
		}

		var executionTime, publicIP string
		startTime := utility.StartTime()

		var instance *civogo.Instance
		resp, err := client.CreateInstance(config)
		if err != nil {
			utility.Error("%s", err)
			os.Exit(1)
		}

		if wait {
			stillCreating := true
			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Prefix = fmt.Sprintf("Creating instance (%s)... ", resp.Hostname)
			s.Start()

			for stillCreating {
				instance, err = client.FindInstance(resp.ID)
				if err != nil {
					utility.Error("%s", err)
					os.Exit(1)
				}
				if instance.Status == "ACTIVE" {
					stillCreating = false
					s.Stop()
				} else {
					time.Sleep(2 * time.Second)
				}
			}
			publicIP = fmt.Sprintf("(%s)", instance.PublicIP)
			executionTime = utility.TrackTime(startTime)
		} else {
			// we look for the created instance to obtain the data that we need
			// like PublicIP
			instance, err = client.FindInstance(resp.ID)
			if err != nil {
				utility.Error("Instance %s", err)
				os.Exit(1)
			}
		}

		if outputFormat == "human" {
			if executionTime != "" {
				fmt.Printf("The instance %s %s has been created in %s\n", utility.Green(instance.Hostname), publicIP, executionTime)
			} else {
				fmt.Printf("The instance %s has been created\n", utility.Green(instance.Hostname))
			}
		} else {
			ow := utility.NewOutputWriter()
			ow.StartLine()
			ow.AppendDataWithLabel("id", resp.ID, "ID")
			ow.AppendDataWithLabel("hostname", resp.Hostname, "Hostname")
			ow.AppendDataWithLabel("size", resp.Size, "Size")
			ow.AppendDataWithLabel("region", resp.Region, "Region")
			ow.AppendDataWithLabel("public_ip", resp.PublicIP, "Public IP")
			ow.AppendDataWithLabel("status", resp.Status, "Status")
			ow.AppendDataWithLabel("network_id", resp.NetworkID, "Network ID")
			ow.AppendDataWithLabel("diskimage_id", resp.SourceID, "Disk image ID")
			ow.AppendDataWithLabel("initial_user", resp.InitialUser, "Initial User")
			ow.AppendDataWithLabel("ssh_key", resp.SSHKey, "SSH Key")
			ow.AppendDataWithLabel("notes", resp.Notes, "Notes")
			ow.AppendDataWithLabel("firewall_id", resp.FirewallID, "Firewall ID")
			ow.AppendDataWithLabel("tags", strings.Join(resp.Tags, " "), "Tags")
			ow.AppendDataWithLabel("script", resp.Script, "Script")
			ow.AppendDataWithLabel("created_at", resp.CreatedAt.Format(time.RFC1123), "Created At")
			ow.AppendDataWithLabel("reverse_dns", resp.ReverseDNS, "Reverse  DNS")
			ow.AppendDataWithLabel("private_ip", resp.PrivateIP, "Private IP")

			if outputFormat == "json" {
				ow.WriteSingleObjectJSON(prettySet)
			} else {
				ow.WriteCustomOutput(outputFields)
			}
		}
	},
}

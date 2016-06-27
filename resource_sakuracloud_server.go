package sakuracloud

import (
	"fmt"
	"github.com/docker/go-units"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/yamamoto-febc/libsacloud/api"
	"github.com/yamamoto-febc/libsacloud/sacloud"
	"time"
)

func resourceSakuraCloudServer() *schema.Resource {
	return &schema.Resource{
		Create: resourceSakuraCloudServerCreate,
		Update: resourceSakuraCloudServerUpdate,
		Read:   resourceSakuraCloudServerRead,
		Delete: resourceSakuraCloudServerDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"core": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			"memory": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			"disks": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"base_interface": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "shared",
			},

			"additional_interfaces": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 3,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"packet_filter_ids": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 4,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"tags": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"zone": &schema.Schema{
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				Description:  "target SakuraCloud zone",
				ValidateFunc: validateStringInWord([]string{"is1a", "is1b", "tk1a", "tk1v"}),
			},
			"mac_addresses": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"base_nw_ipaddress": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"base_nw_dns_servers": &schema.Schema{
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"base_nw_gateway": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"base_nw_address": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
			"base_nw_mask_len": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func resourceSakuraCloudServerCreate(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*api.Client)
	client := c.Clone()
	zone, ok := d.GetOk("zone")
	if ok {
		client.Zone = zone.(string)
	}

	opts := client.Server.New()
	opts.Name = d.Get("name").(string)

	planID, err := client.Product.Server.GetBySpec(d.Get("core").(int), d.Get("memory").(int))
	if err != nil {
		return fmt.Errorf("Invalid server plan.Please change 'core' or 'memory': %s", err)
	}
	opts.SetServerPlanByID(planID.ID.String())

	if hasSharedInterface, ok := d.GetOk("base_interface"); ok {
		switch forceString(hasSharedInterface) {
		case "shared":
			opts.AddPublicNWConnectedParam()
		case "":
			opts.AddEmptyConnectedParam()
		default:
			opts.AddExistsSwitchConnectedParam(forceString(hasSharedInterface))
		}

	} else {
		opts.AddEmptyConnectedParam()
	}

	if interfaces, ok := d.GetOk("additional_interfaces"); ok {
		for _, switchID := range interfaces.([]interface{}) {
			if switchID == nil {
				opts.AddEmptyConnectedParam()
			} else {
				opts.AddExistsSwitchConnectedParam(switchID.(string))
			}
		}
	}

	if description, ok := d.GetOk("description"); ok {
		opts.Description = description.(string)
	}

	if rawTags, ok := d.GetOk("tags"); ok {
		if rawTags != nil {
			opts.Tags = expandStringList(rawTags.([]interface{}))
		}
	}

	server, err := client.Server.Create(opts)
	if err != nil {
		return fmt.Errorf("Failed to create SakuraCloud Server resource: %s", err)
	}

	//connect disk to server
	if _, ok := d.GetOk("disks"); ok {
		rawDisks := d.Get("disks").([]interface{})
		if rawDisks != nil {
			diskIDs := expandStringList(rawDisks)
			for i, diskID := range diskIDs {
				_, err := client.Disk.ConnectToServer(diskID, server.ID)
				if err != nil {
					return fmt.Errorf("Failed to connect SakuraCloud Disk to Server: %s", err)
				}

				// edit disk if server is connected the shared segment
				if i == 0 && len(server.Interfaces) > 0 && server.Interfaces[0].Switch != nil {
					isNeedEditDisk := false
					diskEditConfig := client.Disk.NewCondig()
					if server.Interfaces[0].Switch.Scope == sacloud.ESCopeShared {
						diskEditConfig.SetUserIPAddress(server.Interfaces[0].IPAddress)
						diskEditConfig.SetDefaultRoute(server.Interfaces[0].Switch.Subnet.DefaultRoute)
						diskEditConfig.SetNetworkMaskLen(fmt.Sprintf("%d", server.Interfaces[0].Switch.Subnet.NetworkMaskLen))
						isNeedEditDisk = true
					} else {
						baseIP := forceString(d.Get("base_nw_ipaddress"))
						baseGateway := forceString(d.Get("base_nw_gateway"))
						baseMaskLen := forceString(d.Get("base_nw_mask_len"))

						diskEditConfig.SetUserIPAddress(baseIP)
						diskEditConfig.SetDefaultRoute(baseGateway)
						diskEditConfig.SetNetworkMaskLen(baseMaskLen)

						isNeedEditDisk = baseIP != "" || baseGateway != "" || baseMaskLen != ""
					}

					if isNeedEditDisk {
						_, err := client.Disk.Config(diskID, diskEditConfig)
						if err != nil {
							return fmt.Errorf("Error editting SakuraCloud DiskConfig: %s", err)
						}
					}

				}

			}
		}
	}

	if rawPacketFilterIDs, ok := d.GetOk("packet_filter_ids"); ok {
		packetFilterIDs := rawPacketFilterIDs.([]interface{})
		for i, filterID := range packetFilterIDs {
			strFilterID := ""
			if filterID != nil {
				strFilterID = filterID.(string)
			}
			if server.Interfaces != nil && len(server.Interfaces) > i && strFilterID != "" {
				_, err := client.Interface.ConnectToPacketFilter(server.Interfaces[i].ID, strFilterID)
				if err != nil {
					return fmt.Errorf("Error connecting packet filter: %s", err)
				}
			}
		}
	}
	d.SetId(server.ID)

	//boot
	_, err = client.Server.Boot(d.Id())

	if err != nil {
		return fmt.Errorf("Failed to boot SakuraCloud Server resource: %s", err)
	}
	err = client.Server.SleepUntilUp(d.Id(), 10*time.Minute)
	if err != nil {
		return fmt.Errorf("Failed to boot SakuraCloud Server resource: %s", err)
	}

	return resourceSakuraCloudServerRead(d, meta)

}

func resourceSakuraCloudServerRead(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*api.Client)
	client := c.Clone()
	zone, ok := d.GetOk("zone")
	if ok {
		client.Zone = zone.(string)
	}

	server, err := client.Server.Read(d.Id())
	if err != nil {
		return fmt.Errorf("Couldn't find SakuraCloud Server resource: %s", err)
	}

	d.Set("name", server.Name)
	d.Set("core", server.ServerPlan.CPU)
	d.Set("memory", server.ServerPlan.MemoryMB*units.MiB/units.GiB)
	d.Set("disks", flattenDisks(server.Disks))

	hasSharedInterface := len(server.Interfaces) > 0 && server.Interfaces[0].Switch != nil
	d.Set("base_interface", hasSharedInterface)
	d.Set("additional_interfaces", flattenInterfaces(server.Interfaces))

	d.Set("description", server.Description)
	d.Set("tags", server.Tags)

	d.Set("packet_filter_ids", flattenPacketFilters(server.Interfaces))

	//readonly values
	d.Set("mac_addresses", flattenMacAddresses(server.Interfaces))
	if hasSharedInterface {
		if server.Interfaces[0].Switch.Scope == sacloud.ESCopeShared {
			d.Set("base_nw_ipaddress", server.Interfaces[0].IPAddress)
		} else {
			d.Set("base_nw_ipaddress", server.Interfaces[0].UserIPAddress)
		}
		d.Set("base_nw_dns_servers", server.Zone.Region.NameServers)
		d.Set("base_nw_gateway", server.Interfaces[0].Switch.Subnet.DefaultRoute)
		d.Set("base_nw_address", server.Interfaces[0].Switch.Subnet.NetworkAddress)
		d.Set("base_nw_mask_len", fmt.Sprintf("%d", server.Interfaces[0].Switch.Subnet.NetworkMaskLen))
	} else {
		d.Set("base_nw_ipaddress", "")
		d.Set("base_nw_dns_servers", []string{})
		d.Set("base_nw_gateway", "")
		d.Set("base_nw_address", "")
		d.Set("base_nw_mask_len", "")
	}
	d.Set("zone", client.Zone)

	return nil
}

func resourceSakuraCloudServerUpdate(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*api.Client)
	client := c.Clone()
	zone, ok := d.GetOk("zone")
	if ok {
		client.Zone = zone.(string)
	}

	shutdownFunc := client.Server.Stop

	server, err := client.Server.Read(d.Id())
	if err != nil {
		return fmt.Errorf("Couldn't find SakuraCloud Server resource: %s", err)
	}
	isNeedRestart := false
	isRunning := server.Instance.IsUp()

	if d.HasChange("core") || d.HasChange("memory") {
		// If planID changed , server ID will change.
		planID, err := client.Product.Server.GetBySpec(d.Get("core").(int), d.Get("memory").(int))
		if err != nil {
			return fmt.Errorf("Invalid server plan.Please change 'core' or 'memory': %s", err)
		}
		server.SetServerPlanByID(planID.ID.String())

		isNeedRestart = true
	}

	if d.HasChange("disks") || d.HasChange("base_interface") || d.HasChange("additional_interfaces") {
		isNeedRestart = true
	}

	if isNeedRestart && isRunning {
		// shudown server
		_, err := shutdownFunc(d.Id())
		if err != nil {
			return fmt.Errorf("Error stopping SakuraCloud Server resource: %s", err)
		}

		err = client.Server.SleepUntilDown(d.Id(), 10*time.Minute)
		if err != nil {
			return fmt.Errorf("Error stopping SakuraCloud Server resource: %s", err)
		}
	}

	if d.HasChange("disks") {
		//disconnect all old disks
		for _, disk := range server.Disks {
			_, err := client.Disk.DisconnectFromServer(disk.ID)
			if err != nil {
				return fmt.Errorf("Error disconnecting disk from SakuraCloud Server resource: %s", err)
			}
		}

		rawDisks := d.Get("disks").([]interface{})
		if rawDisks != nil {
			newDisks := expandStringList(rawDisks)
			// connect disks
			for _, diskID := range newDisks {
				_, err := client.Disk.ConnectToServer(diskID, server.ID)
				if err != nil {
					return fmt.Errorf("Error connecting disk to SakuraCloud Server resource: %s", err)
				}
			}

		}

	}

	// NIC
	if d.HasChange("base_interface") {
		sharedNICCon := d.Get("base_interface").(string)
		if sharedNICCon == "shared" {
			client.Interface.ConnectToSharedSegment(server.Interfaces[0].ID)
		} else if sharedNICCon == "" {
			client.Interface.DisconnectFromSwitch(server.Interfaces[0].ID)
		} else {
			client.Interface.ConnectToSwitch(server.Interfaces[0].ID, sharedNICCon)
		}
	}
	if d.HasChange("additional_interfaces") {
		if conf, ok := d.GetOk("additional_interfaces"); ok {
			newNICCount := len(conf.([]interface{}))
			for i, nic := range server.Interfaces {
				if i == 0 {
					continue
				}

				// disconnect exists NIC
				if nic.Switch != nil {
					_, err := client.Interface.DisconnectFromSwitch(nic.ID)
					if err != nil {
						return fmt.Errorf("Error disconnecting NIC from SakuraCloud Switch resource: %s", err)
					}
				}

				//delete NIC
				if i > newNICCount {
					_, err := client.Interface.Delete(nic.ID)
					if err != nil {
						return fmt.Errorf("Error deleting SakuraCloud NIC resource: %s", err)
					}
				}
			}

			for i, s := range conf.([]interface{}) {
				switchID := ""
				if s != nil {
					switchID = s.(string)
				}
				if len(server.Interfaces) <= i+1 {
					//create NIC
					nic := client.Interface.New()
					nic.SetNewServerID(server.ID)
					if switchID != "" {
						nic.SetNewSwitchID(switchID)
					}
					_, err := client.Interface.Create(nic)
					if err != nil {
						return fmt.Errorf("Error creating NIC to SakuraCloud Server resource: %s", err)
					}

				} else {

					if switchID != "" {
						_, err := client.Interface.ConnectToSwitch(server.Interfaces[i+1].ID, switchID)
						if err != nil {
							return fmt.Errorf("Error connecting NIC to SakuraCloud Switch resource: %s", err)
						}
					}
				}
			}

		} else {
			if len(server.Interfaces) > 1 {
				for i, nic := range server.Interfaces {
					if i == 0 {
						continue
					}

					_, err := client.Interface.Delete(nic.ID)
					if err != nil {
						return fmt.Errorf("Error deleting SakuraCloud NIC resource: %s", err)
					}
				}
			}
		}

	}

	// change Plan
	if d.HasChange("core") || d.HasChange("memory") {
		server, err := client.Server.ChangePlan(d.Id(), server.ServerPlan.ID.String())
		if err != nil {
			return fmt.Errorf("Error changing SakuraCloud ServerPlan : %s", err)
		}
		d.SetId(server.ID)
	}

	if d.HasChange("name") {
		server.Name = d.Get("name").(string)
	}
	if d.HasChange("description") {
		if description, ok := d.GetOk("description"); ok {
			server.Description = description.(string)
		} else {
			server.Description = ""
		}
	}

	if d.HasChange("tags") {
		rawTags := d.Get("tags").([]interface{})
		if rawTags != nil {
			server.Tags = expandStringList(rawTags)
		}
	}

	server, err = client.Server.Update(d.Id(), server)
	if err != nil {
		return fmt.Errorf("Error updating SakuraCloud Server resource: %s", err)
	}
	d.SetId(server.ID)

	if d.HasChange("packet_filter_ids") {
		if rawPacketFilterIDs, ok := d.GetOk("packet_filter_ids"); ok {
			packetFilterIDs := rawPacketFilterIDs.([]interface{})
			for i, filterID := range packetFilterIDs {
				strFilterID := ""
				if filterID != nil {
					strFilterID = filterID.(string)
				}
				if server.Interfaces != nil && len(server.Interfaces) > i {
					if server.Interfaces[i].PacketFilter != nil {
						_, err := client.Interface.DisconnectFromPacketFilter(server.Interfaces[i].ID)
						if err != nil {
							return fmt.Errorf("Error disconnecting packet filter: %s", err)
						}
					}

					if strFilterID != "" {
						_, err := client.Interface.ConnectToPacketFilter(server.Interfaces[i].ID, filterID.(string))
						if err != nil {
							return fmt.Errorf("Error connecting packet filter: %s", err)
						}
					}
				}
			}

			if len(server.Interfaces) > len(packetFilterIDs) {
				i := len(packetFilterIDs)
				for i < len(server.Interfaces) {
					if server.Interfaces[i].PacketFilter != nil {
						_, err := client.Interface.DisconnectFromPacketFilter(server.Interfaces[i].ID)
						if err != nil {
							return fmt.Errorf("Error disconnecting packet filter: %s", err)
						}
					}

					i++
				}

			}

		} else {
			if server.Interfaces != nil {
				for _, i := range server.Interfaces {
					if i.PacketFilter != nil {
						_, err := client.Interface.DisconnectFromPacketFilter(i.ID)
						if err != nil {
							return fmt.Errorf("Error disconnecting packet filter: %s", err)
						}
					}
				}
			}
		}
	}

	if isNeedRestart && isRunning {
		_, err := client.Server.Boot(d.Id())
		if err != nil {
			return fmt.Errorf("Error booting SakuraCloud Server resource: %s", err)
		}

		err = client.Server.SleepUntilUp(d.Id(), 10*time.Minute)
		if err != nil {
			return fmt.Errorf("Error booting SakuraCloud Server resource: %s", err)
		}

	}

	return resourceSakuraCloudServerRead(d, meta)

}

func resourceSakuraCloudServerDelete(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*api.Client)
	client := c.Clone()
	zone, ok := d.GetOk("zone")
	if ok {
		client.Zone = zone.(string)
	}

	server, err := client.Server.Read(d.Id())
	if err != nil {
		return fmt.Errorf("Couldn't find SakuraCloud Server resource: %s", err)
	}

	if server.Instance.IsUp() {
		_, err := client.Server.Stop(d.Id())
		if err != nil {
			return fmt.Errorf("Error stopping SakuraCloud Server resource: %s", err)
		}

		err = client.Server.SleepUntilDown(d.Id(), 10*time.Minute)
		if err != nil {
			return fmt.Errorf("Error stopping SakuraCloud Server resource: %s", err)
		}
	}

	_, err = client.Server.Delete(d.Id())

	if err != nil {
		return fmt.Errorf("Error deleting SakuraCloud Server resource: %s", err)
	}

	return nil

}

package azurerm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceArmVirtualMachineExtensions() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmVirtualMachineExtensionsCreate,
		Read:   resourceArmVirtualMachineExtensionsRead,
		Update: resourceArmVirtualMachineExtensionsCreate,
		Delete: resourceArmVirtualMachineExtensionsDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": locationSchema(),

			"resource_group_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"virtual_machine_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"publisher": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"type": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"type_handler_version": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"auto_upgrade_minor_version": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"settings": &schema.Schema{
				Type:             schema.TypeString,
				Optional:         true,
				ValidateFunc:     validateJsonString,
				DiffSuppressFunc: suppressDiffVirtualMachineExtensionSettings,
			},

			// due to the sensitive nature, these are not returned by the API
			"protected_settings": &schema.Schema{
				Type:             schema.TypeString,
				Optional:         true,
				Sensitive:        true,
				ValidateFunc:     validateJsonString,
				DiffSuppressFunc: suppressDiffVirtualMachineExtensionSettings,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceArmVirtualMachineExtensionsCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).vmExtensionClient

	name := d.Get("name").(string)
	location := d.Get("location").(string)
	vmName := d.Get("virtual_machine_name").(string)
	resGroup := d.Get("resource_group_name").(string)
	publisher := d.Get("publisher").(string)
	extensionType := d.Get("type").(string)
	typeHandlerVersion := d.Get("type_handler_version").(string)
	autoUpgradeMinor := d.Get("auto_upgrade_minor_version").(bool)
	tags := d.Get("tags").(map[string]interface{})

	extension := compute.VirtualMachineExtension{
		Location: &location,
		VirtualMachineExtensionProperties: &compute.VirtualMachineExtensionProperties{
			Publisher:               &publisher,
			Type:                    &extensionType,
			TypeHandlerVersion:      &typeHandlerVersion,
			AutoUpgradeMinorVersion: &autoUpgradeMinor,
		},
		Tags: expandTags(tags),
	}

	if settingsString := d.Get("settings").(string); settingsString != "" {
		settings, err := expandArmVirtualMachineExtensionSettings(settingsString)
		if err != nil {
			return fmt.Errorf("unable to parse settings: %s", err)
		}
		extension.VirtualMachineExtensionProperties.Settings = &settings
	}

	if protectedSettingsString := d.Get("protected_settings").(string); protectedSettingsString != "" {
		protectedSettings, err := expandArmVirtualMachineExtensionSettings(protectedSettingsString)
		if err != nil {
			return fmt.Errorf("unable to parse protected_settings: %s", err)
		}
		extension.VirtualMachineExtensionProperties.ProtectedSettings = &protectedSettings
	}

	_, err := client.CreateOrUpdate(resGroup, vmName, name, extension, make(chan struct{}))
	if err != nil {
		return err
	}

	read, err := client.Get(resGroup, vmName, name, "")
	if err != nil {
		return err
	}

	if read.ID == nil {
		return fmt.Errorf("Cannot read  Virtual Machine Extension %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmVirtualMachineExtensionsRead(d, meta)
}

func resourceArmVirtualMachineExtensionsRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).vmExtensionClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	vmName := id.Path["virtualMachines"]
	name := id.Path["extensions"]

	resp, err := client.Get(resGroup, vmName, name, "")

	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error making Read request on Virtual Machine Extension %s: %s", name, err)
	}

	d.Set("name", resp.Name)
	d.Set("location", azureRMNormalizeLocation(*resp.Location))
	d.Set("virtual_machine_name", vmName)
	d.Set("resource_group_name", resGroup)
	d.Set("publisher", resp.VirtualMachineExtensionProperties.Publisher)
	d.Set("type", resp.VirtualMachineExtensionProperties.Type)
	d.Set("type_handler_version", resp.VirtualMachineExtensionProperties.TypeHandlerVersion)
	d.Set("auto_upgrade_minor_version", resp.VirtualMachineExtensionProperties.AutoUpgradeMinorVersion)

	if resp.VirtualMachineExtensionProperties.Settings != nil {
		settings, err := flattenArmVirtualMachineExtensionSettings(*resp.VirtualMachineExtensionProperties.Settings)
		if err != nil {
			return fmt.Errorf("unable to parse settings from response: %s", err)
		}
		d.Set("settings", settings)
	}

	flattenAndSetTags(d, resp.Tags)

	return nil
}

func resourceArmVirtualMachineExtensionsDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient).vmExtensionClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}
	resGroup := id.ResourceGroup
	name := id.Path["extensions"]
	vmName := id.Path["virtualMachines"]

	_, err = client.Delete(resGroup, vmName, name, make(chan struct{}))

	return nil
}

func expandArmVirtualMachineExtensionSettings(jsonString string) (map[string]interface{}, error) {
	var result map[string]interface{}

	err := json.Unmarshal([]byte(jsonString), &result)

	return result, err
}

func flattenArmVirtualMachineExtensionSettings(settingsMap map[string]interface{}) (string, error) {
	result, err := json.Marshal(settingsMap)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

func suppressDiffVirtualMachineExtensionSettings(k, old, new string, d *schema.ResourceData) bool {
	oldMap, err := expandArmVirtualMachineExtensionSettings(old)
	if err != nil {
		return false
	}

	newMap, err := expandArmVirtualMachineExtensionSettings(new)
	if err != nil {
		return false
	}

	return reflect.DeepEqual(oldMap, newMap)
}

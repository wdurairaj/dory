/*
(c) Copyright 2017 Hewlett Packard Enterprise Development LP

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

package dockervol

import (
	"fmt"
	"nimblestorage/pkg/connectivity"
	"nimblestorage/pkg/docker/dockerlt"
	"nimblestorage/pkg/util"
	"strings"
)

const (
	//ActivateURI is /Plugin.Activate
	ActivateURI = "/Plugin.Activate"
	//CreateURI is /VolumeDriver.Create
	CreateURI = "/VolumeDriver.Create"
	//ListURI is /VolumeDriver.List
	ListURI = "/VolumeDriver.List"
	//CapabilitiesURI is /VolumeDriver.Capabilities
	CapabilitiesURI = "/VolumeDriver.Capabilities"
	//RemoveURI is /VolumeDriver.Remove
	RemoveURI = "/VolumeDriver.Remove"
	//MountURI is /VolumeDriver.Mount
	MountURI = "/VolumeDriver.Mount"
	//UnmountURI is /VolumeDriver.Unmount
	UnmountURI = "/VolumeDriver.Unmount"
	//GetURI is /VolumeDriver.Get
	GetURI = "/VolumeDriver.Get"
	//NotFound describes the beginning of the not found error message
	NotFound = "Unable to find"

	defaultSocketPath = "/run/docker/plugins/nimble.sock"
)

//Options  for volumedriver
type Options struct {
	SocketPath                   string
	StripK8sFromOptions          bool
	LogFilePath                  string
	Debug                        bool
	CreateVolumes                bool
	ListOfStorageResourceOptions []string
	FactorForConversion          int
}

//DockerVolumePlugin is the client to a specific docker volume plugin
type DockerVolumePlugin struct {
	stripK8sOpts                 bool
	client                       *connectivity.Client
	ListOfStorageResourceOptions []string
	FactorForConversion          int
}

//Errorer describes the ability get the embedded error
type Errorer interface {
	getErr() string
}

//Request is the basic request to use when talking to the driver
type Request struct {
	Name string                 `json:"Name,omitempty"`
	Opts map[string]interface{} `json:"Opts,omitempty"`
}

//MountRequest is used to mount and unmount volumes
type MountRequest struct {
	Name string `json:"Name,omitempty"`
	ID   string `json:"ID,omitempty"`
}

//MountResponse is returned from the volume driver
type MountResponse struct {
	Mountpoint string `json:"Mountpoint,omitempty"`
	Err        string `json:"Err,omitempty"`
}

func (g *MountResponse) getErr() string {
	return g.Err
}

//GetResponse is returned from the volume driver
type GetResponse struct {
	Volume DockerVolume `json:"Volume,omitempty"`
	Err    string       `json:"Err,omitempty"`
}

func (g *GetResponse) getErr() string {
	return g.Err
}

//GetListResponse is returned from the volume driver list request
type GetListResponse struct {
	Volumes []DockerVolume `json:"Volumes,omitempty"`
	Err     string         `json:"Err,omitempty"`
}

func (g *GetListResponse) getErr() string {
	return g.Err
}

//DockerVolume represents the details about a docker volume
type DockerVolume struct {
	Name       string                 `json:"Name,omitempty"`
	Mountpoint string                 `json:"Mountpoint,omitempty"`
	Status     map[string]interface{} `json:"Status,omitempty"`
}

//CapResponse describes the capabilities of the plugin
type CapResponse struct {
	Capabilities PluginCapabilities `json:"Capabilities,omitempty"`
}

//PluginCapabilities includes the scope of the plugin
type PluginCapabilities struct {
	Scope string `json:"Scope,omitempty"`
}

// NewDockerVolumePlugin creates a DockerVolumePlugin which can be used to communicate with
// a Docker Volume Plugin.  socketPath is the full path to the location of the socket file for the nimble volume plugin.
// stripK8sFromOptions indicates if k8s namespace should be stripped fromoptions.
func NewDockerVolumePlugin(options *Options) (*DockerVolumePlugin, error) {
	var err error
	if !strings.HasPrefix(options.SocketPath, "/") {
		// this is a v2 plugin, so we need to find its socket file
		options.SocketPath, err = getV2PluginSocket(options.SocketPath, "")
	}
	if err != nil {
		return nil, err
	}

	if options.SocketPath == "" {
		options.SocketPath = defaultSocketPath
	}
	dvp := &DockerVolumePlugin{
		stripK8sOpts: options.StripK8sFromOptions,
		client:       connectivity.NewSocketClient(options.SocketPath),
		ListOfStorageResourceOptions: options.ListOfStorageResourceOptions,
		FactorForConversion:          options.FactorForConversion,
	}

	// test connectivity
	_, err = dvp.Capabilities()
	if err != nil {
		return dvp, err
	}

	return dvp, nil

}

type empty struct{}

//Capabilities returns the capabilities supported by the plugin
func (dvp *DockerVolumePlugin) Capabilities() (*CapResponse, error) {
	var req = &empty{}
	var res = &CapResponse{}

	err := dvp.driverRun(&connectivity.Request{
		Action:        "POST",
		Path:          CapabilitiesURI,
		Payload:       req,
		Response:      res,
		ResponseError: res})
	if err != nil {
		util.LogInfo.Printf("unable to get Capabilities - %s\n", err.Error())
		return nil, err
	}

	util.LogDebug.Printf("returning %#v", res)
	return res, nil
}

//Get a docker volume by docker name returning the response from the driver
func (dvp *DockerVolumePlugin) Get(name string) (*GetResponse, error) {
	var req = &Request{Name: name}
	var res = &GetResponse{}

	err := dvp.driverRun(&connectivity.Request{
		Action:        "POST",
		Path:          GetURI,
		Payload:       req,
		Response:      res,
		ResponseError: res})
	if err != nil {
		util.LogInfo.Printf("unable to get docker volume using %s - %s\n", name, err.Error())
		return nil, err
	}

	if err = driverErrorCheck(res); err != nil {
		util.LogInfo.Printf("unable to get docker volume using %s - %s\n", name, err.Error())
		return nil, err
	}
	util.LogDebug.Printf("returning %#v", res)
	return res, nil
}

//List the docker volumes returning the response from the driver
func (dvp *DockerVolumePlugin) List() (*GetListResponse, error) {
	var req = &Request{}
	var res = &GetListResponse{}

	err := dvp.driverRun(&connectivity.Request{
		Action:        "POST",
		Path:          ListURI,
		Payload:       req,
		Response:      res,
		ResponseError: res})
	if err != nil {
		util.LogInfo.Printf("unable to list docker volumes - %s\n", err.Error())
		return nil, err
	}

	if err = driverErrorCheck(res); err != nil {
		util.LogInfo.Printf("unable to list docker volumes - %s\n", err.Error())
		return nil, err
	}
	util.LogDebug.Printf("returning %#v", res)
	return res, nil
}

//Create a docker volume returning the docker volume name
func (dvp *DockerVolumePlugin) Create(name string, options map[string]interface{}) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	for key := range options {
		if key == "name" || (dvp.stripK8sOpts && strings.HasPrefix(key, "kubernetes.io")) {
			delete(options, key)
		}
	}
	var req = &Request{Name: name, Opts: options}
	var res = &GetResponse{}

	err := dvp.driverRun(&connectivity.Request{
		Action:        "POST",
		Path:          CreateURI,
		Payload:       req,
		Response:      res,
		ResponseError: res})
	if err != nil {
		util.LogError.Printf("unable to create docker volume using %v & %v - %s\n", name, options, err.Error())
		return "", err
	}

	if err = driverErrorCheck(res); err != nil {
		util.LogError.Printf("unable to create docker volume using %v & %v - %s\n", name, options, err.Error())
		return "", err
	}

	return res.Volume.Name, nil
}

//Mount attaches and mounts a nimble volume returning the path
func (dvp *DockerVolumePlugin) Mount(name, mountID string) (string, error) {
	m, err := dvp.mounter(name, mountID, MountURI)
	if err != nil {
		return "", err
	}
	return m, nil
}

//Unmount unmounts and detaches nimble volume
func (dvp *DockerVolumePlugin) Unmount(name, mountID string) error {
	_, err := dvp.mounter(name, mountID, UnmountURI)
	if err != nil {
		return err
	}
	return nil
}

//Delete calls the delete function of the plugin
func (dvp *DockerVolumePlugin) Delete(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	var req = &Request{Name: name}
	var res = &GetResponse{}

	err := dvp.driverRun(&connectivity.Request{
		Action:        "POST",
		Path:          RemoveURI,
		Payload:       req,
		Response:      res,
		ResponseError: res})
	if err != nil {
		util.LogError.Printf("%s failed %v - %s\n", RemoveURI, name, err.Error())
		return err
	}

	if err = driverErrorCheck(res); err != nil {
		util.LogError.Printf("%s failed %v - %s\n", RemoveURI, name, err.Error())
		return err
	}

	return nil
}

func (dvp *DockerVolumePlugin) mounter(name, mountID string, path string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	var req = &MountRequest{Name: name, ID: mountID}
	var res = &MountResponse{}

	err := dvp.driverRun(&connectivity.Request{
		Action:        "POST",
		Path:          path,
		Payload:       req,
		Response:      res,
		ResponseError: res})
	if err != nil {
		util.LogError.Printf("%s failed %v & %v - %s\n", path, name, mountID, err.Error())
		return "", err
	}

	if err = driverErrorCheck(res); err != nil {
		util.LogError.Printf("%s failed %v & %v - %s\n", path, name, mountID, err.Error())
		return "", err
	}

	return res.Mountpoint, nil
}

func (dvp *DockerVolumePlugin) driverRun(r *connectivity.Request) error {
	return dvp.client.DoJSON(r)
}

func driverErrorCheck(e Errorer) error {
	if e.getErr() != "" {
		return fmt.Errorf(e.getErr())
	}
	return nil
}

// name is the name of the docker volume plugin.  dockerSocket is the full path to the docker socket.  The default is used if an empty string is passed.
func getV2PluginSocket(name, dockerSocket string) (string, error) {
	c := dockerlt.NewDockerClient(dockerSocket)
	plugins, err := c.PluginsGet()

	if err != nil {
		return "", fmt.Errorf("failed to get V2 plugins from docker. error=%s", err.Error())
	}

	for _, plugin := range plugins {
		if strings.Compare(name, plugin.Name) == 0 || strings.Compare(fmt.Sprintf("%s:latest", name), plugin.Name) == 0 {
			if !plugin.Enabled {
				return fmt.Sprintf("/run/docker/plugins/%s/%s", plugin.ID, plugin.Config.Interface.Socket), fmt.Errorf("found Docker V2 Plugin named %s, but it is disabled", name)
			}
			return fmt.Sprintf("/run/docker/plugins/%s/%s", plugin.ID, plugin.Config.Interface.Socket), nil
		}
	}

	return "", fmt.Errorf("unable to find V2 plugin named %s", name)
}

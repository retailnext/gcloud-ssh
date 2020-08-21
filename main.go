// Copyright (c) 2020 RetailNext, Inc.
// This software may be modified and distributed under the terms
// of the MIT license. See the LICENSE file for details.
// All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
)

var (
	errorHasIdentityFile = errors.New("Has identity file")
)

// finds the project, zone and instance name that belongs to a networkIP
func findInstance(computeService *compute.Service, projects, zones []string, networkIP string) (string, string, string, error) {
	for _, project := range projects {
		projectZones := zones
		if len(projectZones) == 0 {
			// If no zones are specified list all available zones
			zoneListCall := computeService.Zones.List(project)
			zoneList, err := zoneListCall.Do()
			if err != nil {
				return "", "", "", err
			}
			for _, zone := range zoneList.Items {
				projectZones = append(projectZones, zone.Name)
			}
		}

		for _, zone := range projectZones {
			instanceListCall := computeService.Instances.List(project, zone)
			instanceListCall.Filter("(status = RUNNING)")
			instanceList, err := instanceListCall.Do()
			if err != nil {
				return "", "", "", err
			}

			for _, instance := range instanceList.Items {
				for _, ni := range instance.NetworkInterfaces {
					if ni.NetworkIP == networkIP {
						log.Printf("Found network IP: %s in zone: %s with name: %s", networkIP, zone, instance.Name)
						return instance.Name, zone, project, nil
					}
				}
			}
		}
	}

	return "", "", "", fmt.Errorf("Not found networkIP: %v", networkIP)
}

func runGCloudSSH(ar AnsibleRun) error {
	cmd := exec.Command("gcloud",
		"compute",
		"ssh",
		"--quiet",
		"--tunnel-through-iap",
		"--project", ar.Project,
		"--zone", ar.Zone, ar.Destination,
		"--command", ar.Command,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runGCloudSCP(ar AnsibleRun) error {
	cmd := exec.Command("gcloud",
		"compute",
		"scp",
		"--quiet",
		"--tunnel-through-iap",
		"--project", ar.Project,
		"--zone", ar.Zone, ar.Source, ar.Destination,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runSystemSCP(args []string) error {
	log.Println("Running system-scp with args:", args)
	cmd := exec.Command("system-scp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type AnsibleRun struct {
	Command     string
	Source      string
	Destination string
	Zone        string
	Project     string

	Options []string
}

func ParseAnsibleArgs(args []string) (AnsibleRun, error) {
	result := AnsibleRun{}
	commands := []string{}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-c":
			i++
			result.Command = args[i]
			continue
		case "-o":
			i++
			result.Options = append(result.Options, args[i])
			continue
		default:
			if arg[0] == '-' {
				continue
			}
		}

		if result.Destination == "" {
			result.Destination = arg
		} else {
			commands = append(commands, arg)
		}
	}

	if result.Destination == "" {
		return result, fmt.Errorf("Empty destination")
	}
	if result.Command == "" {
		result.Command = strings.Join(commands, " ")
	}
	if result.Command == "" {
		return result, fmt.Errorf("Empty command")
	}
	log.Printf("Parsed ansible ssh: %#+v", result)
	return result, nil
}

func ParseAnsibleSCP(args []string) (AnsibleRun, error) {
	result := AnsibleRun{}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-i":
			return result, errorHasIdentityFile
		case "-o":
			i++
			result.Options = append(result.Options, args[i])
			continue
		default:
			if arg[0] == '-' {
				continue
			}
		}
		if result.Source == "" {
			result.Source = arg
			continue
		}
		result.Destination = arg
	}

	if result.Destination == "" {
		return result, fmt.Errorf("Empty destination")
	}
	if result.Source == "" {
		return result, fmt.Errorf("Empty source")
	}
	log.Printf("Parsed ansible scp: %#+v", result)
	return result, nil
}

func ExtractIP(str string) string {
	// SCP destination is [xxx]:yyy
	parts := strings.Split(str, ":")
	wrapped := parts[0]
	if strings.Index(wrapped, "[") == 0 {
		return wrapped[1 : len(wrapped)-1]
	}
	return parts[0]
}

func updateWithInstanceName(projects, zones []string, ansible *AnsibleRun) error {
	ctx := context.Background()
	client, err := google.DefaultClient(ctx, compute.ComputeScope)
	if err != nil {
		return err
	}

	networkIP := ExtractIP(ansible.Destination)

	computeService, err := compute.New(client)
	if err != nil {
		return err
	}
	instanceName, zone, project, err := findInstance(computeService, projects, zones, networkIP)
	if err != nil {
		return err
	}

	if strings.Index(ansible.Destination, "[") == 0 {
		ansible.Destination = strings.Replace(ansible.Destination, "["+networkIP+"]", instanceName, -1)
	} else {
		ansible.Destination = strings.Replace(ansible.Destination, networkIP, instanceName, -1)
	}
	ansible.Zone = zone
	ansible.Project = project

	return nil
}

func setupLogger() func() {
	f, err := os.OpenFile("/var/log/gcloud-ssh.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(f)

	return func() {
		f.Close()
	}
}

// Get env var or default
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Get env var or default
func getEnvList(key string, fallback []string) []string {
	if value, ok := os.LookupEnv(key); ok {
		if value != "" {
			return strings.Split(value, ",")
		}
	}
	return fallback
}

func parseAndRun(doSCP bool, projects, zones []string) error {
	if doSCP {
		// Check if we have to run system's scp command
		ansible, err := ParseAnsibleSCP(os.Args)
		if err != nil {
			if err == errorHasIdentityFile {
				err = runSystemSCP(os.Args[1:])
			}
			return err
		}

		// Running Cloud SCP
		err = updateWithInstanceName(projects, zones, &ansible)
		if err != nil {
			return err
		}
		runGCloudSCP(ansible)
	} else {
		ansible, err := ParseAnsibleArgs(os.Args)
		if err != nil {
			return err
		}

		err = updateWithInstanceName(projects, zones, &ansible)
		if err != nil {
			return err
		}
		runGCloudSSH(ansible)
	}
	return nil
}

func main() {
	closeLogger := setupLogger()
	defer closeLogger()

	doSCP, _ := strconv.ParseBool(getEnv("DO_SCP", "false"))
	zones := getEnvList("GCLOUD_SSH_ZONES", []string{})
	projects := getEnvList("GCLOUD_SSH_PROJECTS", []string{})

	if len(projects) == 0 {
		ctx := context.Background()
		credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
		if err != nil {
			log.Fatal(err)
		}
		projects = append(projects, credentials.ProjectID)
	}
	log.Printf("Starting with zones: %v, projects: %v, doSCP: %v", zones, projects, doSCP)

	err := parseAndRun(doSCP, projects, zones)
	if err != nil {
		log.Println(err)
		fmt.Println(err)
	}
}

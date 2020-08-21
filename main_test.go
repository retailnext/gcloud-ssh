// Copyright (c) 2020 RetailNext, Inc.
// This software may be modified and distributed under the terms
// of the MIT license. See the LICENSE file for details.
// All rights reserved.

package main

import (
	"fmt"
	"testing"
)

const (
	argState    = 1
	startState  = 2
	quotesState = 3
)

func parseCommandLine(command string) ([]string, error) {
	var args []string
	var quote byte = '"'
	state := startState
	current := ""
	escapeNext := true
	for i := 0; i < len(command); i++ {
		c := command[i]

		if state == quotesState {
			if c != quote {
				current += string(c)
			} else {
				args = append(args, current)
				current = ""
				state = startState
			}
			continue
		}

		if escapeNext {
			current += string(c)
			escapeNext = false
			continue
		}

		if c == '\\' {
			escapeNext = true
			continue
		}

		if c == '"' || c == '\'' {
			state = quotesState
			quote = c
			continue
		}

		if state == argState {
			if c == ' ' || c == '\t' {
				args = append(args, current)
				current = ""
				state = startState
			} else {
				current += string(c)
			}
			continue
		}

		if c != ' ' && c != '\t' {
			state = argState
			current += string(c)
		}
	}

	if state == quotesState {
		return []string{}, fmt.Errorf("Unclosed quote in command line: %s", command)
	}

	if current != "" {
		args = append(args, current)
	}

	return args, nil
}

func TestSSH(t *testing.T) {
	// gcloud compute ssh --tunnel-through-iap --quiet --zone "us-central1-a" "andy-awx-managed-1" --command "ls"
	args, err := parseCommandLine(`-C -o ControlMaster=auto -o ControlPersist=60s -o StrictHostKeyChecking=no -o KbdInteractiveAuthentication=no -o PreferredAuthentications=gssapi-with-mic,gssapi-keyex,hostbased,publickey -o PasswordAuthentication=no -o User="andy_retailnext_net" -o ConnectTimeout=10 -o ControlPath=/tmp/awx_2826_wrtrtczl/cp/3c85463f3f 172.16.0.12 /bin/sh -c '/usr/bin/python3 && sleep 0'`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ParseAnsibleArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	expected := "/usr/bin/python3 && sleep 0"
	if a.Command != expected {
		t.Fatalf("'%v' != '%v'", a.Command, expected)
	}

	args2, err := parseCommandLine(`-C -o ControlMaster=auto -o ControlPersist=60s -o StrictHostKeyChecking=no -o KbdInteractiveAuthentication=no -o PreferredAuthentications=gssapi-with-mic,gssapi-keyex,hostbased,publickey -o PasswordAuthentication=no -o User="sa_111069622966946909314" -o ConnectTimeout=10 -o ControlPath=/tmp/awx_2848_uq4itrrn/cp/811b91f774 172.16.0.11 dd of=/home/sa_111069622966946909314/.ansible/tmp/ansible-tmp-1596502037.623799-215726-161858982386430/AnsiballZ_setup.py bs=65536`)
	if err != nil {
		t.Fatal(err)
	}
	a, err = ParseAnsibleArgs(args2)
	if err != nil {
		t.Fatal(err)
	}
	expected = "dd of=/home/sa_111069622966946909314/.ansible/tmp/ansible-tmp-1596502037.623799-215726-161858982386430/AnsiballZ_setup.py bs=65536"
	if a.Command != expected {
		t.Fatalf("'%v' != '%v'", a.Command, expected)
	}
	if a.Destination != "172.16.0.11" {
		t.Fatalf("'%v' != '%v'", a.Destination, "172.16.0.11")
	}
	if a.Source != "" {
		t.Fatalf("destination must be empty, found '%v'", a.Source)
	}
}

func TestSCP(t *testing.T) {
	args3, err := parseCommandLine(`-C -o ControlMaster=auto -o ControlPersist=60s -o StrictHostKeyChecking=no -o KbdInteractiveAuthentication=no -o PreferredAuthentications=gssapi-with-mic,gssapi-keyex,hostbased,publickey -o PasswordAuthentication=no -o User="sa_111069622966946909314" -o ConnectTimeout=10 -o ControlPath=/tmp/awx_2850_mcg_tln7/cp/811b91f774 /var/lib/awx/.ansible/tmp/ansible-local-216033jdy7a18f/tmpja9h4a0t [172.16.0.11]:/home/sa_111069622966946909314/.ansible/tmp/ansible-tmp-1596502613.2008872-216047-259015317780472/AnsiballZ_setup.py`)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ParseAnsibleSCP(args3)
	if err != nil {
		t.Fatal(err)
	}

	if a.Command != "" {
		t.Fatalf("command must be empty, found '%v'", a.Command)
	}
	expected := `/var/lib/awx/.ansible/tmp/ansible-local-216033jdy7a18f/tmpja9h4a0t`
	if a.Source != expected {
		t.Fatalf("'%v' != '%v'", a.Source, expected)
	}
	expected = `[172.16.0.11]:/home/sa_111069622966946909314/.ansible/tmp/ansible-tmp-1596502613.2008872-216047-259015317780472/AnsiballZ_setup.py`
	if a.Destination != expected {
		t.Fatalf("'%v' != '%v'", a.Destination, expected)
	}

	expected = "172.16.0.11"
	ip := ExtractIP(a.Destination)
	if ip != expected {
		t.Fatalf("'%v' != '%v'", ip, expected)
	}

	ip = ExtractIP(expected)
	if ip != expected {
		t.Fatalf("'%v' != '%v'", ip, expected)
	}
}

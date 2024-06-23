# THIS IS A WORK IN PROGRESS

I reserve the right to rewrite/rebase the primary branch or even change it entirely.

You've been warned.

# Pulumi Shenanigans

Why "yet another thing to get up to speed with" do we really need another automation tool to build stuff? Perhaps not. But in a nutshell this is intended to act like a quickstart or essentially solve the bootstrap problem of "I have nothing how can I get up to speed and running something locally in short order?".

For now it is libvirt only but it is written in pulumi using golang so the intent is we can expand provider support later rather easily. The tool also assumes that airgap is the "default" generally in that we want to enable the ability to build setups when on an airplane at 30 000 feet if necessary.

To use download pulumi and golang go here, this readme will not cover installing dependencies:
- [pulumi](https://www.pulumi.com/docs/get-started/install/) Tested version: v3.94.0
- [go](https://go.dev/dl/) Tested version: 1.21.6

Note that there is an *.envrc* file that will use nix and setup all dependencies for you if you use *direnv*. Other versions of golang or pulumi may work but are untested.

## First time setup

If you have never ran *pulumi*, first login locally to save run state. Unless you want to use their cloud offering and sign up for an account in which case go ahead and login to that instead. This readme presumes we have logged in locally already, state is stored in `$HOME/.pulumi` in this case.

"Login" to the local setup:
``` sh
pulumi login --local
```

## Select a predefined stack

There are a number of stacks defined already. Feel free to use an existing stack or to create you own. For the readme we will simply use a simple rke2 setup that installs the rancher management server.

To select this stack run:

```sh
pulumi stack select -cs rancher
```

## Using a what a stack built

Once you have brought a stack up via:

```
pulumi up -yf
```

There will be a number of files in the `~/.cache/shenanigans` directory as well as stack output variables to aide in usage.

### To ssh into a vm


Once the stack is up there will be a ssh-config file in your XDG_CACHE dirt. You can use that to ssh into the vm group(s) like so:

```sh
ssh -F $(find ~/.cache/shenanigans -name config -path '*/upstream/*ssh/*') vm0 uptime
 20:55:57  up   0:08,  0 users,  load average: 1.47, 1.32, 0.73
```

Once the stack is up you can alternatively use the stack output, note examples are using the rancher config which has a name of upstream:

```sh
ssh -F $(pulumi stack output upstream:ssh-config) vm0 uptime
 21:00:40  up   0:13,  0 users,  load average: 0.95, 1.25, 0.87
```

You should also have a local kubeconfig file you can use with most k8s utilities. Similar to the above usage is the same:
```sh
KUBECONFIG=$(find ~/.cache/shenanigans -name config -path '*/upstream/*kube/*') kubectl get nodes
NAME                         STATUS   ROLES                       AGE     VERSION
luckily-clean-piranha        Ready    control-plane,etcd,master   8m32s   v1.27.14+rke2r1
violently-crucial-tortoise   Ready    <none>                      6m5s    v1.27.14+rke2r1
```

Or via the stack output:
```sh
KUBECONFIG=$(pulumi stack output upstream:kube-config) kubectl get nodes
NAME                         STATUS   ROLES                       AGE     VERSION
luckily-clean-piranha        Ready    control-plane,etcd,master   10m     v1.27.14+rke2r1
violently-crucial-tortoise   Ready    <none>                      7m41s   v1.27.14+rke2r1
```

## k3s/rke2 specific

### Figure out what version of k3s/rke2 to install

You can abuse this crazy oneliner to find all the versions of k3s you could possibly abuse.

```sh
curl --silent 'https://api.github.com/repos/k3s-io/k3s/git/refs/tags' | jq -r '.[] | select(.ref) | .ref' | sed 's|refs/tags/||g' | grep -Ev rc | sort -u
```

Note: for k3s as we use the newer token setup that is similar to rke2, versions older than 1.26 won't work without building in support/feature detection for the old "prime control plane node generates token" setup. Pull requests welcome.

Or for rke2:
```sh
curl --silent 'https://api.github.com/repos/k3s-io/k3s/git/refs/tags' | jq -r '.[] | select(.ref) | .ref' | sed 's|refs/tags/||g' | grep -Ev rc | sort -u
```

Simply add *| grep MAJOR.MINOR* to the end of the above commands to find the versions available for both.

# Internals and more explanation of why?

## Why is this thing written in golang using pulumi vs <someothertool>?

In a nutshell the general reason is to keep dependencies low from the perspective of the user. The overall approach is:
- Pulumi driven via golang decides "when/how" to do stuff from the config
- How to do stuff remotely is done via a remote binary also built off the same code base that is copied to the built vm's to do work that traditionally would be done via chef/puppet/ansible.
- FUTURE: This lets the config work be unit tested. (lots to do yet here)
- FUTURE: Build a shenanigans binary/wrapper for all this junk that can talk to/from the pulumi binary that is built and the remote binary via say grpc?

The other advantage this has is the tooling can itself use golang apis to interface with say helm/kubectl to validate setup vs to run external commands like kubectl/helm. This drastically improves the situation on automation. Leaving pulumi to setup infra only we can then have local and remote commands do the rest of our work more easily and in an easily validated way

### Why Pulumi?

In a nutshell the author finds terraform to be rather limiting in its inability to dynamically define and update its dependency DAG as well as modules in terraform across providers tend to "leak" and end up being more work than they are worth. The terraform solution to rerun the setup is... at best a hack, and at worst a terrible user experience that invites more problems. Additionally the hcl dsl ends up rather limiting in its ability to abstract over things like providers and modules. Having a "real" programming language means we can use the same approaches as one would when programming in general versus being beholden to a rather limited DSL in HCL.

As pulumi can use terraform plugins and use programming languages to abstract over them via an api, it seems the best option currently available.

### Why golang?

I'd rather not use golang if I'm honest, but its the only language option that makes sense right now. The goals for this repo are:
- To make UX simpler by reducing installation requirements, python and its myriad of packaging is not fun to handle, and as I've written a grand total of 30 lines of javascript in my life, typescript/javascript are kinda out. I know nothing of .net either so golang it is even if its not my favorite.
- The other reason as noted above, is to share code between pulumi and "what runs on the vm's to do setup"

### How does this work internally?

The general idea is (in a traditional setup that presumes we run a vm with some os or another then "run stuff" to configure it at runtime):
- Pulumi sets up vm's
- It also uses go to build a remote binary that is copied to those vm's
- At the appropriate time runs that binary with the info needed to do its work

The other approach is that there are "groups" that allow setup of disparate complexity. As an example that means that a Pulumi.stack.yaml would define groups of arbitrary complexity that depend upon each other.

This enables us to setup say, k8s, then install something onto it, build more vm's after the first k8s in parallel like a downstream k8s cluster or more, and then join those downstreams to rancher.

How this all knows about itself is encoded in the remote binary and the binary pulumi builds. So each group is basically in the end an argument passed to a binary. Example for k8s that would boil down to something like so:

`remote k8s --worker --server 1.2.3.4`

This would be executed on any k8s agent node to do any configuration for a worker node talking to a control plane at ip 1.2.3.4. It will either succeed or fail. It has no timeout, that is left to pulumi it either passes or fails. If it needs to retry remotely that logic is done there.

TODO: I should define a way to pass inputs such as all vm's configuration between groups vs hard coding things.
TODO: Should I build a local binary to run things as well? e.g. for rancher installation just use the kube api and helm to install helm charts vs running helm commands or kubectl. Then I can unit test crap this runs the same way. Future mitch problem.

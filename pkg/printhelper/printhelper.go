package printhelper

import (
	"github.com/loft-sh/loftctl/v2/pkg/log"
	"github.com/mgutz/ansi"
)

const passwordChangedHint = "(has been changed)"

func PrintDNSConfiguration(host string, log log.Logger) {
	log.WriteString(`

###################################     DNS CONFIGURATION REQUIRED     ##################################

Create a DNS A-record for ` + host + ` with the EXTERNAL-IP of your nginx-ingress controller.
To find this EXTERNAL-IP, run the following command and look at the output:

> kubectl get services -n ingress-nginx
                                                     |---------------|
NAME                       TYPE           CLUSTER-IP | EXTERNAL-IP   |  PORT(S)                      AGE
ingress-nginx-controller   LoadBalancer   10.0.0.244 | XX.XXX.XXX.XX |  80:30984/TCP,443:31758/TCP   19m
                                                     |^^^^^^^^^^^^^^^|

EXTERNAL-IP may be 'pending' for a while until your cloud provider has created a new load balancer.

#########################################################################################################

The command will wait until loft is reachable under the host. You can also abort and use port-forwarding instead
by running 'loft start' again.

`)
}

func PrintSuccessMessageLocalInstall(password, localPort string, log log.Logger) {
	url := "https://localhost:" + localPort

	if password == "" {
		password = passwordChangedHint
	}

	log.WriteString(`

##########################   LOGIN   ############################

Username: ` + ansi.Color("admin", "green+b") + `
Password: ` + ansi.Color(password, "green+b") + `  # Change via UI or via: ` + ansi.Color("loft reset password", "green+b") + `

Login via UI:  ` + ansi.Color(url, "green+b") + `
Login via CLI: ` + ansi.Color(`loft login --insecure `+url, "green+b") + `

!!! You must accept the untrusted certificate in your browser !!!

#################################################################

Loft was successfully installed and port-forwarding has been started.
If you stop this command, run 'loft start' again to restart port-forwarding.

Thanks for using Loft!
`)
}

func PrintSuccessMessageRemoteInstall(host, password string, log log.Logger) {
	url := "https://" + host

	if password == "" {
		password = passwordChangedHint
	}

	log.WriteString(`


##########################   LOGIN   ############################

Username: ` + ansi.Color("admin", "green+b") + `
Password: ` + ansi.Color(password, "green+b") + `  # Change via UI or via: ` + ansi.Color("loft reset password", "green+b") + `

Login via UI:  ` + ansi.Color(url, "green+b") + `
Login via CLI: ` + ansi.Color(`loft login --insecure `+url, "green+b") + `

!!! You must accept the untrusted certificate in your browser !!!

Follow this guide to add a valid certificate: https://loft.sh/docs/administration/ssl

#################################################################

Loft was successfully installed and can now be reached at: ` + url + `

Thanks for using Loft!
`)
}

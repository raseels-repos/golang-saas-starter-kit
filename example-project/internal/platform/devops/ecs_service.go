package devops

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/sethgrid/pester"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
	"github.com/aws/aws-sdk-go/aws/session"
)

// EcsServiceTaskInit allows newly spun up ECS Service Tasks to register their public IP with Route 53.
func EcsServiceTaskInit(log *log.Logger, awsSession *session.Session) error {
	ecsClusterName := os.Getenv("ECS_CLUSTER")
	ecsServiceName := os.Getenv("ECS_SERVICE")

	// If both env variables are empty, this instance of the services is not running on AWS ECS.
	if ecsClusterName == "" && ecsServiceName == "" {
		return nil
	}

	res, err := pester.Get("http://169.254.170.2/v2/metadata")
	if err != nil {
		fmt.Println("http://169.254.170.2/v2/metadata failed", err.Error())
	} else {
		dat, _ := ioutil.ReadAll(res.Body)
		res.Body.Close()
		fmt.Println("http://169.254.170.2/v2/metadata, OK", string(dat))
	}

	var zoneArecNames = map[string][]string{}
	if v := os.Getenv("ROUTE53_ZONES"); v != "" {
		dat, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil {
			return errors.Wrapf(err, "failed to base64 URL decode zones")
		}

		err = json.Unmarshal(dat, &zoneArecNames)
		if err != nil {
			return errors.Wrapf(err, "failed to json decode zones - %s", string(dat))
		}
	}

	var registerServiceTasks bool
	if v := os.Getenv("ROUTE53_UPDATE_TASK_IPS"); v != "" {
		var err error
		registerServiceTasks, err = strconv.ParseBool(v)
		if err != nil {
			return errors.Wrapf(err, "failed to parse ROUTE53_UPDATE_TASK_IPS value '%s' to bool", v)
		}
	}

	if registerServiceTasks {
		if err := RegisterEcsServiceTasksRoute53(log, awsSession, ecsClusterName, ecsServiceName, zoneArecNames); err != nil {
			return err
		}
	}

	return nil
}

// EcsServiceTaskTaskShutdown allows ECS Service Tasks that are spinning down to deregister their public IP with Route 53.
func EcsServiceTaskTaskShutdown(log *log.Logger, awsSession *session.Session) error {
	// TODO: Should lookup the IP for the current running task and remove that specific IP.
	// 		 For now just run the init since it removes all non-running tasks.
	return EcsServiceTaskInit(log, awsSession)
}

// RegisterEcsServiceTasksRoute53 registers the public IPs for a ECS Service Task with Route 53.
func RegisterEcsServiceTasksRoute53(log *log.Logger, awsSession *session.Session, ecsClusterName, ecsServiceName string, zoneArecNames map[string][]string) error {
	var networkInterfaceIds []string
	for a := 0; a <= 3; a++ {
		svc := ecs.New(awsSession)

		serviceRes, err := svc.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  aws.String(ecsClusterName),
			Services: []*string{aws.String(ecsServiceName)},
		})
		if err != nil {
			return errors.Wrapf(err, "failed to describe service '%s'", ecsServiceName)
		}
		service := serviceRes.Services[0]

		servceTaskRes, err := svc.ListTasks(&ecs.ListTasksInput{
			Cluster:     aws.String(ecsClusterName),
			ServiceName: aws.String(ecsServiceName),
			DesiredStatus: aws.String("RUNNING"),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to list tasks for cluster '%s' service '%s'", ecsClusterName, ecsServiceName)
		}

		taskRes, err := svc.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: aws.String(ecsClusterName),
			Tasks:   servceTaskRes.TaskArns,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to describe %d tasks for cluster '%s'", len(servceTaskRes.TaskArns), ecsClusterName)
		}

		for _, t := range taskRes.Tasks {
			if *t.TaskDefinitionArn != *service.TaskDefinition || *t.DesiredStatus != "RUNNING" {
				continue
			}

			if t.Attachments == nil {
				continue
			}

			for _, c := range t.Containers {
				if *c.Name != ecsServiceName {
					continue
				}

				if c.NetworkInterfaces == nil || len(c.NetworkInterfaces) == 0 || c.NetworkInterfaces[0].AttachmentId == nil {
					continue
				}

				for _, a := range t.Attachments {
					if a.Details == nil ||  *a.Id != *c.NetworkInterfaces[0].AttachmentId {
						continue
					}

					for _, ad := range a.Details {
						if ad.Name != nil && *ad.Name == "networkInterfaceId" {
							networkInterfaceIds = append(networkInterfaceIds, *ad.Value)
						}
					}
				}

				break
			}
		}

		if len(networkInterfaceIds) > 0 {
			log.Printf("Found %d network interface IDs.\n",  len(networkInterfaceIds))
			break
		}

		// Found no network interfaces, try again.
		log.Println( "Found no network interfaces.")
		time.Sleep((time.Duration(a) * time.Second * 10) * time.Duration(a))
	}

	log.Println("Get public IPs for network interface IDs.")
	var publicIps []string
	for a := 0; a <= 3; a++ {
		svc := ec2.New(awsSession)

		log.Println("\t\tDescribe network interfaces.")
		res, err := svc.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: aws.StringSlice(networkInterfaceIds),
		})
		if err != nil {
			return errors.Wrap(err, "failed to describe network interfaces")
		}

		for _, ni := range res.NetworkInterfaces {
			if ni.Association == nil || ni.Association.PublicIp == nil {
				continue
			}
			publicIps = append(publicIps, *ni.Association.PublicIp)
		}

		if len(publicIps) > 0 {
			log.Printf("Found %d public IPs.\n", len(publicIps))
			break
		}

		// Found no public IPs, try again.
		log.Println( "Found no public IPs.")
		time.Sleep((time.Duration(a) * time.Second * 10) * time.Duration(a))
	}

	if len(publicIps) > 0 {
		log.Println("Update public IPs for hosted zones.")

		svc := route53.New(awsSession)

		// Public IPs to be served as round robin.
		log.Printf("\tPublic IPs:\n")
		rrs := []*route53.ResourceRecord{}
		for _, ip := range publicIps {
			log.Printf("\t\t%s\n", ip)
			rrs = append(rrs, &route53.ResourceRecord{Value: aws.String(ip)})
		}

		for zoneId, aNames := range zoneArecNames {
			log.Printf("\tChange zone '%s'.\n", zoneId)

			input := &route53.ChangeResourceRecordSetsInput{
				ChangeBatch: &route53.ChangeBatch{
					Changes: []*route53.Change{},
				},
				HostedZoneId: aws.String(zoneId),
			}

			// Add all the A record names with the same set of public IPs.
			for _, aName := range aNames {
				log.Printf("\t\tAdd A record for '%s'.\n", aName)

				input.ChangeBatch.Changes = append(input.ChangeBatch.Changes, &route53.Change{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(aName),
						ResourceRecords: rrs,
						TTL:             aws.Int64(60),
						Type:            aws.String("A"),
					},
				})
			}

			_, err := svc.ChangeResourceRecordSets(input)
			if err != nil {
				return errors.Wrapf(err, "failed to update A records for zone '%s'", zoneId)
			}
		}

		log.Printf("DNS entries updated.\n")
	}

	return nil
}


/*
http://169.254.170.2/v2/metadata,

{
	"Cluster": "arn:aws:ecs:us-west-2:888955683113:cluster/example-project-dev",
	"TaskARN": "arn:aws:ecs:us-west-2:888955683113:task/700e38dd-dec5-4201-b711-c04a51feef8a",
	"Family": "web-api",
	"Revision": "113",
	"DesiredStatus": "RUNNING",
	"KnownStatus": "RUNNING",
	"Containers": [{
		"DockerId": "c786dfdf6510b20294832ccbc3d66e6f1f915a4a79ead2588aa760a6365c839a",
		"Name": "datadog-agent",
		"DockerName": "ecs-web-api-113-datadog-agent-d884dee0c79af1fb6400",
		"Image": "datadog/agent:latest",
		"ImageID": "sha256:233c75f21f71838a59d478472d021be7006e752da6a70a11f77cf185c1050737",
		"Labels": {
			"com.amazonaws.ecs.cluster": "arn:aws:ecs:us-west-2:888955683113:cluster/example-project-dev",
			"com.amazonaws.ecs.container-name": "datadog-agent",
			"com.amazonaws.ecs.task-arn": "arn:aws:ecs:us-west-2:888955683113:task/700e38dd-dec5-4201-b711-c04a51feef8a",
			"com.amazonaws.ecs.task-definition-family": "web-api",
			"com.amazonaws.ecs.task-definition-version": "113"
		},
		"DesiredStatus": "RUNNING",
		"KnownStatus": "STOPPED",
		"ExitCode": 1,
		"Limits": {
			"CPU": 128,
			"Memory": 0
		},
		"CreatedAt": "2019-07-11T05:36:54.135666318Z",
		"StartedAt": "2019-07-11T05:36:54.481305866Z",
		"FinishedAt": "2019-07-11T05:36:54.863742829Z",
		"Type": "NORMAL",
		"Networks": [{
			"NetworkMode": "awsvpc",
			"IPv4Addresses": ["172.31.62.204"]
		}],
		"Volumes": [{
			"DockerName": "0960558c657c6e79d43e0e55f4ff259a97d78f58d9ad0d738e74495f4ba3cb06",
			"Source": "/var/lib/docker/volumes/0960558c657c6e79d43e0e55f4ff259a97d78f58d9ad0d738e74495f4ba3cb06/_data",
			"Destination": "/etc/datadog-agent"
		}, {
			"DockerName": "7a103f880857a1c2947e4a1bfff48efd25d24943a2d6a6e4dd86fa9dab3f10f0",
			"Source": "/var/lib/docker/volumes/7a103f880857a1c2947e4a1bfff48efd25d24943a2d6a6e4dd86fa9dab3f10f0/_data",
			"Destination": "/tmp"
		}, {
			"DockerName": "c88c03366eadb5d9da27708919e77ac5f8e0877c3dbb32c80580cb22e5811c00",
			"Source": "/var/lib/docker/volumes/c88c03366eadb5d9da27708919e77ac5f8e0877c3dbb32c80580cb22e5811c00/_data",
			"Destination": "/var/log/datadog"
		}, {
			"DockerName": "df97387f6ccc34c023055ef8a34a41e9d1edde4715c1849f1460683d31749539",
			"Source": "/var/lib/docker/volumes/df97387f6ccc34c023055ef8a34a41e9d1edde4715c1849f1460683d31749539/_data",
			"Destination": "/var/run/s6"
		}]
	}, {
		"DockerId": "ab6bd869e675f64122a33a74da9183b304bbc60b649a15d0d83ebc48eeafdd76",
		"Name": "~internal~ecs~pause",
		"DockerName": "ecs-web-api-113-internalecspause-aab99b88b9ddadb0c701",
		"Image": "fg-proxy:tinyproxy",
		"ImageID": "",
		"Labels": {
			"com.amazonaws.ecs.cluster": "arn:aws:ecs:us-west-2:888955683113:cluster/example-project-dev",
			"com.amazonaws.ecs.container-name": "~internal~ecs~pause",
			"com.amazonaws.ecs.task-arn": "arn:aws:ecs:us-west-2:888955683113:task/700e38dd-dec5-4201-b711-c04a51feef8a",
			"com.amazonaws.ecs.task-definition-family": "web-api",
			"com.amazonaws.ecs.task-definition-version": "113"
		},
		"DesiredStatus": "RESOURCES_PROVISIONED",
		"KnownStatus": "RESOURCES_PROVISIONED",
		"Limits": {
			"CPU": 0,
			"Memory": 0
		},
		"CreatedAt": "2019-07-11T05:36:34.896093577Z",
		"StartedAt": "2019-07-11T05:36:35.302359045Z",
		"Type": "CNI_PAUSE",
		"Networks": [{
			"NetworkMode": "awsvpc",
			"IPv4Addresses": ["172.31.62.204"]
		}]
	}, {
		"DockerId": "07bce50839fc992393799457811e4a0ac56979b2164c7aec6e66b40162ae3119",
		"Name": "web-api-dev",
		"DockerName": "ecs-web-api-113-web-api-dev-ceefbfb4dba2a6e05900",
		"Image": "888955683113.dkr.ecr.us-west-2.amazonaws.com/example-project:dev-web-api",
		"ImageID": "sha256:cf793de01311ac4e5e32c76cb4625f6600ec8017c726e99e28ec2199d4af599b",
		"Labels": {
			"com.amazonaws.ecs.cluster": "arn:aws:ecs:us-west-2:888955683113:cluster/example-project-dev",
			"com.amazonaws.ecs.container-name": "web-api-dev",
			"com.amazonaws.ecs.task-arn": "arn:aws:ecs:us-west-2:888955683113:task/700e38dd-dec5-4201-b711-c04a51feef8a",
			"com.amazonaws.ecs.task-definition-family": "web-api",
			"com.amazonaws.ecs.task-definition-version": "113",
			"com.datadoghq.ad.check_names": "[\"web-api-dev\"]",
			"com.datadoghq.ad.init_configs": "[{}]",
			"com.datadoghq.ad.instances": "[{\"host\": \"%%host%%\", \"port\": 80}]",
			"com.datadoghq.ad.logs": "[{\"source\": \"docker\", \"service\": \"web-api-dev\", \"service_name\": \"web-api\", \"cluster\": \"example-project-dev\", \"env\": \"dev\"}]"
		},
		"DesiredStatus": "RUNNING",
		"KnownStatus": "RUNNING",
		"Limits": {
			"CPU": 128,
			"Memory": 0
		},
		"CreatedAt": "2019-07-11T05:36:42.417547421Z",
		"StartedAt": "2019-07-11T05:36:53.88095717Z",
		"Type": "NORMAL",
		"Networks": [{
			"NetworkMode": "awsvpc",
			"IPv4Addresses": ["172.31.62.204"]
		}],
		"Health": {}
	}],
	"Limits": {
		"CPU": 0.5,
		"Memory": 2048
	},
	"PullStartedAt": "2019-07-11T05:36:35.407114703Z",
	"PullStoppedAt": "2019-07-11T05:36:54.128398742Z"
}
 */
package devops

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
)

// EcsServiceTaskInit allows newly spun up ECS Service Tasks to register their public IP with Route 53.
func EcsServiceTaskInit(log *log.Logger, awsSession *session.Session) error {
	if awsSession == nil {
		return nil
	}

	ecsClusterName := os.Getenv("ECS_CLUSTER")
	ecsServiceName := os.Getenv("ECS_SERVICE")

	// If both env variables are empty, this instance of the services is not running on AWS ECS.
	if ecsClusterName == "" && ecsServiceName == "" {
		return nil
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
	if awsSession == nil {
		return nil
	}

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
			Cluster:       aws.String(ecsClusterName),
			ServiceName:   aws.String(ecsServiceName),
			DesiredStatus: aws.String("RUNNING"),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to list tasks for cluster '%s' service '%s'", ecsClusterName, ecsServiceName)
		}

		if len(servceTaskRes.TaskArns) == 0 {
			continue
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
					if a.Details == nil || *a.Id != *c.NetworkInterfaces[0].AttachmentId {
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
			log.Printf("Found %d network interface IDs.\n", len(networkInterfaceIds))
			break
		}

		// Found no network interfaces, try again.
		log.Println("Found no network interfaces.")
		time.Sleep((time.Duration(a) * time.Second * 10) * time.Duration(a))
	}

	if len(networkInterfaceIds) == 0 {
		return errors.New("Unable to update public IPs. No network interfaces found.")
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
		log.Println("Found no public IPs.")
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
			addedNames := make(map[string]bool)
			for _, aName := range aNames {
				if addedNames[aName] {
					continue
				}
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
				addedNames[aName] = true
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

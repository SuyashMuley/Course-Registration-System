# infra/terraform

Owner: Santosh.

Infrastructure-as-code for AWS: VPC, ALB, EC2 app instances, RDS Postgres (Multi-AZ), ElastiCache Redis, the second region for E7, and the SNS topic and CloudWatch alarms.

Note: confirm what the AWS Learner Lab allows (regions, RDS Multi-AZ, ElastiCache, cross-region replicas) before building. Never commit `*.tfstate` or `*.tfvars` (see .gitignore).

# infra/terraform

Owner: Santosh.

Infrastructure-as-code for AWS: VPC, ALB, EC2 app instances, RDS Postgres (Multi-AZ), ElastiCache Redis, the second region for E7, and the SNS topic and CloudWatch alarms.

AWS Learner Lab limits (checked Nov 22, 2026): only us-east-1 and us-west-2. RDS: burstable classes nano to medium, gp2 up to 100 GB, on-demand only, Enhanced Monitoring off (also turn off Performance Insights, which the console enables by default). Multi-AZ works. Read replicas are blocked (`rds:CreateDBInstanceReadReplica` is denied), so the second region has no database. ElastiCache: listed in the Lab readme with no extra restrictions; confirm a small node (e.g., `cache.t3.micro`, node-based, not Serverless) works during the trial launch. Never commit `*.tfstate` or `*.tfvars` (see .gitignore).

package ssh

import "github.com/shopware/shopware-cli/internal/deployment"

type Backend = deployment.Backend
type Candidate = deployment.Candidate
type CandidateProvider = deployment.CandidateProvider
type CreateOptions = deployment.CreateOptions
type Deployment = deployment.Deployment
type DeploymentActivator = deployment.DeploymentActivator
type DeploymentLogs = deployment.DeploymentLogs
type Rollout = deployment.Rollout
type RolloutHistory = deployment.RolloutHistory

var ErrNotSupported = deployment.ErrNotSupported

type Initializer = deployment.Initializer

package ssh

import "github.com/shopware/shopware-cli/internal/deployment"

type Backend = deployment.Backend
type Candidate = deployment.Candidate
type CandidateProvider = deployment.CandidateProvider
type CreateOptions = deployment.CreateOptions
type Deployment = deployment.Deployment
type DeploymentActivator = deployment.DeploymentActivator
type DeploymentLogs = deployment.DeploymentLogs
type DeploymentPruneHostResult = deployment.DeploymentPruneHostResult
type DeploymentPruneOptions = deployment.DeploymentPruneOptions
type DeploymentPruneResult = deployment.DeploymentPruneResult
type DeploymentPruner = deployment.DeploymentPruner
type Rollout = deployment.Rollout
type RolloutHistory = deployment.RolloutHistory

var ErrNotSupported = deployment.ErrNotSupported

type Preflight = deployment.Preflight
type PreflightAction = deployment.PreflightAction
type PreflightOptions = deployment.PreflightOptions
type PreflightTarget = deployment.PreflightTarget
type PreflightProvider = deployment.PreflightProvider

const PreflightRollout = deployment.PreflightRollout
const PreflightRollback = deployment.PreflightRollback

var WritePreflight = deployment.WritePreflight

type Initializer = deployment.Initializer

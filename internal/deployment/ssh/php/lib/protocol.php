<?php

// Line protocol with the Go client: status lines go to stdout, authorizations
// arrive on stdin. EOF never authorizes anything.
function deploymentSend(string $message): void
{
    echo "$message\n";
    fflush(STDOUT);
}

function deploymentExpect(string $message, string $error): void
{
    if (rtrim((string) fgets(STDIN), "\r\n") !== $message) {
        throw new RuntimeException($error);
    }
}

function deploymentCoordinated(array $input): bool
{
    return ($input['coordinated'] ?? false) === true;
}

/** Multi-host rollouts prepare only after every host has staged the release. */
function deploymentAwaitPreparation(array $input, string $id): void
{
    if (deploymentCoordinated($input)) {
        deploymentSend("STAGED $id");
        deploymentExpect("PREPARE $id", 'Client did not authorize preparation');
    }
}

/** Keep the deployment lock until the coordinator has finished the whole cohort. */
function deploymentAwaitCohort(array $input): void
{
    if (deploymentCoordinated($input)) {
        fgets(STDIN);
    }
}

function deploymentActivationResult(array $input, array $metadata, bool $unchanged): void
{
    if (($input['action'] ?? '') === 'activate' || deploymentCoordinated($input)) {
        deploymentSend(json_encode([
            'reference' => $metadata['reference'],
            'deployment' => ['reference' => $metadata['deployment']],
            'deployed_at' => $metadata['deployed_at'],
            'active' => true,
            'unchanged' => $unchanged,
        ], JSON_THROW_ON_ERROR));
    }
}

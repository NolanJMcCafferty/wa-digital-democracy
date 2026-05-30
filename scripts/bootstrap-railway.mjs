#!/usr/bin/env node
import process from "node:process";

const API_URL = "https://backboard.railway.com/graphql/v2";
const REPO = env("RAILWAY_GITHUB_REPO") || env("GITHUB_REPOSITORY") || "NolanJMcCafferty/wa-digital-democracy";
const PROJECT_NAME = env("RAILWAY_PROJECT_NAME") || "wa-digital-democracy";
const PROJECT_DESCRIPTION = env("RAILWAY_PROJECT_DESCRIPTION") || "Washington Digital Democracy hosted demo";
const WORKSPACE_ID = emptyToNull(env("RAILWAY_WORKSPACE_ID"));
const token = env("RAILWAY_API_TOKEN") || env("RAILWAY_TOKEN");

const argv = process.argv.slice(2);
const mode = argv.find((arg) => !arg.startsWith("--")) || "apply";
const dryRun = argv.includes("--dry-run");
const deploy = argv.includes("--deploy") || mode === "deploy";
const skipDeploy = argv.includes("--skip-deploy");
const deployCommitSha = env("RAILWAY_DEPLOY_COMMIT_SHA") || env("GITHUB_SHA") || "";
const environmentFilter = new Set((env("RAILWAY_ENVIRONMENT_FILTER") || "").split(",").map((value) => value.trim()).filter(Boolean));

if (["-h", "--help", "help"].includes(mode)) {
  printHelp();
  process.exit(0);
}
if (!["apply", "deploy"].includes(mode)) {
  fail(`Unknown mode '${mode}'. Use 'apply' or 'deploy'.`);
}
if (!token && !dryRun) {
  fail("Set RAILWAY_API_TOKEN or RAILWAY_TOKEN to a Railway account/workspace token.");
}

const allEnvSpecs = [
  {
    name: "production",
    branch: env("RAILWAY_PRODUCTION_BRANCH") || "main",
    appEnv: "production",
    suffix: "PRODUCTION",
    bucketDefault: "wa-dd-raw-prod",
    siteUrlDefault: env("PRODUCTION_WEB_PUBLIC_URL") || env("WEB_PUBLIC_URL") || "",
  },
  {
    name: "staging",
    branch: env("RAILWAY_STAGING_BRANCH") || "staging",
    appEnv: "staging",
    suffix: "STAGING",
    bucketDefault: "wa-dd-raw-staging",
    siteUrlDefault: env("STAGING_WEB_PUBLIC_URL") || "",
  },
];

const envSpecs = environmentFilter.size > 0 ? allEnvSpecs.filter((spec) => environmentFilter.has(spec.name)) : allEnvSpecs;
if (envSpecs.length === 0) {
  fail(`RAILWAY_ENVIRONMENT_FILTER did not match any known environment: ${Array.from(environmentFilter).join(", ")}`);
}

const serviceSpecs = [
  {
    name: "postgis",
    source: { repo: REPO },
    branchByEnv: true,
    config: {
      rootDirectory: "/",
      dockerfilePath: "infra/railway/postgis/Dockerfile",
      railwayConfigFile: "/infra/railway/config/postgis.railway.json",
      restartPolicyType: "ON_FAILURE",
      restartPolicyMaxRetries: 10,
    },
    volume: {
      mountPath: "/var/lib/postgresql/data",
    },
    publicDomain: false,
  },
  {
    name: "api",
    source: { repo: REPO },
    branchByEnv: true,
    config: {
      rootDirectory: "/",
      dockerfilePath: "Dockerfile",
      railwayConfigFile: "/railway.json",
      startCommand: "wa-dd-api",
      healthcheckPath: "/healthz",
      healthcheckTimeout: 120,
      restartPolicyType: "ON_FAILURE",
      restartPolicyMaxRetries: 10,
    },
    publicDomain: true,
    targetPort: 8080,
  },
  {
    name: "web",
    source: { repo: REPO },
    branchByEnv: true,
    config: {
      rootDirectory: "apps/web",
      dockerfilePath: "Dockerfile",
      railwayConfigFile: "/apps/web/railway.json",
      startCommand: "/bin/sh -c './node_modules/.bin/next start -H 0.0.0.0 -p \"${PORT:-3000}\"'",
      healthcheckPath: "/healthz",
      healthcheckTimeout: 60,
      restartPolicyType: "ON_FAILURE",
      restartPolicyMaxRetries: 10,
    },
    publicDomain: true,
    targetPort: 3000,
  },
  {
    name: "migrate",
    source: { repo: REPO },
    branchByEnv: true,
    deploymentTrigger: false,
    config: {
      rootDirectory: "/",
      dockerfilePath: "Dockerfile",
      railwayConfigFile: "/infra/railway/config/migrate.railway.json",
      startCommand: "/bin/sh -c 'for i in $(seq 1 60); do goose -dir /app/db/migrations postgres \"$DATABASE_URL\" up && exit 0; echo \"goose migration attempt $i failed; retrying in 5s\" >&2; sleep 5; done; goose -dir /app/db/migrations postgres \"$DATABASE_URL\" up'",
      restartPolicyType: "NEVER",
    },
    publicDomain: false,
  },
  {
    name: "daily",
    source: { repo: REPO },
    branchByEnv: true,
    config: {
      rootDirectory: "/",
      dockerfilePath: "Dockerfile",
      railwayConfigFile: "/infra/railway/config/daily.railway.json",
      startCommand: "/bin/sh -c 'wa-dd daily --biennium \"${BIENNIUM:-2025-26}\"'",
      cronSchedule: env("DAILY_CRON") || "30 3 * * *",
      restartPolicyType: "NEVER",
    },
    publicDomain: false,
  },
];

const serviceNameSet = new Set(serviceSpecs.map((svc) => svc.name));

main().catch((error) => {
  console.error(error?.stack || error?.message || String(error));
  process.exit(1);
});

async function main() {
  console.log(`${dryRun ? "[dry-run] " : ""}Railway bootstrap for ${PROJECT_NAME} (${REPO})`);
  if (dryRun) {
    for (const spec of envSpecs) {
      console.log(`- would reconcile ${spec.name} from branch ${spec.branch}`);
    }
    return;
  }

  const project = await ensureProject();
  const productionSpec = allEnvSpecs.find((spec) => spec.name === "production");
  const production = await ensureEnvironment(project, productionSpec);
  let staging = null;
  if (envSpecs.some((spec) => spec.name === "staging")) {
    const stagingSpec = allEnvSpecs.find((spec) => spec.name === "staging");
    staging = await ensureEnvironment(project, stagingSpec, production.id);
  }
  const environments = new Map([[production.name, production]]);
  if (staging) environments.set(staging.name, staging);

  const services = new Map();
  for (const spec of serviceSpecs) {
    const service = await ensureService(project, spec);
    services.set(spec.name, service);
  }

  for (const envSpec of envSpecs) {
    const railwayEnv = environments.get(envSpec.name);
    console.log(`\nReconciling environment: ${envSpec.name}`);

    for (const serviceSpec of serviceSpecs) {
      const service = services.get(serviceSpec.name);
      await updateServiceInstance(service, railwayEnv, serviceSpec);
      if (serviceSpec.deploymentTrigger === false) {
        await removeDeploymentTriggers(project, railwayEnv, service);
      } else {
        await ensureDeploymentTrigger(project, railwayEnv, service, envSpec.branch);
      }
    }

    await ensurePostgisVolume(project, railwayEnv, services.get("postgis"), serviceSpecs[0], envSpec);

    const domains = await ensureDomains(project, railwayEnv, services);
    await upsertEnvironmentVariables(project, railwayEnv, envSpec, domains, services);

    if (deploy && !skipDeploy) {
      await deployEnvironment(railwayEnv, services);
    }
  }

  console.log("\nRailway bootstrap complete.");
}

async function ensureProject() {
  const existing = await findProjectByName(PROJECT_NAME);
  if (existing) {
    console.log(`Project exists: ${existing.name} (${existing.id})`);
    return existing;
  }
  console.log(`Creating project: ${PROJECT_NAME}`);
  const input = {
    name: PROJECT_NAME,
    description: PROJECT_DESCRIPTION,
    isPublic: false,
    defaultEnvironmentName: "production",
  };
  if (WORKSPACE_ID) input.workspaceId = WORKSPACE_ID;
  return gql(
    `mutation projectCreate($input: ProjectCreateInput!) {
      projectCreate(input: $input) { id name description }
    }`,
    { input },
    "projectCreate",
  );
}

async function findProjectByName(name) {
  const data = await gql(
    `query projects($workspaceId: String) {
      projects(workspaceId: $workspaceId) {
        edges { node { id name description } }
      }
    }`,
    { workspaceId: WORKSPACE_ID },
  );
  return data.projects.edges.map((edge) => edge.node).find((project) => project.name === name) || null;
}

async function ensureEnvironment(project, spec, sourceEnvironmentId = null) {
  const existing = await findEnvironment(project.id, spec.name);
  if (existing) {
    console.log(`Environment exists: ${spec.name} (${existing.id})`);
    return existing;
  }
  console.log(`Creating environment: ${spec.name}`);
  const input = {
    projectId: project.id,
    name: spec.name,
    skipInitialDeploys: true,
    stageInitialChanges: false,
  };
  if (sourceEnvironmentId) input.sourceEnvironmentId = sourceEnvironmentId;
  return gql(
    `mutation environmentCreate($input: EnvironmentCreateInput!) {
      environmentCreate(input: $input) { id name }
    }`,
    { input },
    "environmentCreate",
  );
}

async function findEnvironment(projectId, name) {
  const data = await gql(
    `query environments($projectId: String!, $isEphemeral: Boolean) {
      environments(projectId: $projectId, isEphemeral: $isEphemeral) {
        edges { node { id name } }
      }
    }`,
    { projectId, isEphemeral: false },
  );
  return data.environments.edges.map((edge) => edge.node).find((environment) => environment.name === name) || null;
}

async function ensureService(project, spec) {
  const services = await listServices(project.id);
  const existing = services.find((service) => service.name === spec.name);
  if (existing) {
    console.log(`Service exists: ${spec.name} (${existing.id})`);
    return existing;
  }
  console.log(`Creating service: ${spec.name}`);
  const input = {
    projectId: project.id,
    name: spec.name,
    source: spec.source,
    branch: envSpecs[0].branch,
  };
  return gql(
    `mutation serviceCreate($input: ServiceCreateInput!) {
      serviceCreate(input: $input) { id name projectId }
    }`,
    { input },
    "serviceCreate",
  );
}

async function listServices(projectId) {
  const data = await gql(
    `query project($id: String!) {
      project(id: $id) {
        services { edges { node { id name projectId } } }
      }
    }`,
    { id: projectId },
  );
  return data.project.services.edges.map((edge) => edge.node);
}

async function removeDeploymentTriggers(project, railwayEnv, service) {
  const triggers = await listDeploymentTriggers(project.id, railwayEnv.id, service.id);
  for (const trigger of triggers.filter((item) => item.repository === REPO)) {
    try {
      await gql(
        `mutation deploymentTriggerDelete($id: String!) {
          deploymentTriggerDelete(id: $id)
        }`,
        { id: trigger.id },
        "deploymentTriggerDelete",
      );
      console.log(`  ${service.name}: removed deploy trigger for ${trigger.branch}`);
    } catch (error) {
      console.warn(`  ${service.name}: could not remove deploy trigger for ${trigger.branch} (${error.message})`);
    }
  }
}

async function ensureDeploymentTrigger(project, railwayEnv, service, branch) {
  const triggers = await listDeploymentTriggers(project.id, railwayEnv.id, service.id);
  const existing = triggers.find((trigger) => trigger.repository === REPO && trigger.branch === branch);
  if (existing) {
    console.log(`  ${service.name}: deploy trigger exists for ${branch}`);
    return existing;
  }

  for (const trigger of triggers.filter((item) => item.repository === REPO && item.branch !== branch)) {
    try {
      await gql(
        `mutation deploymentTriggerDelete($id: String!) {
          deploymentTriggerDelete(id: $id)
        }`,
        { id: trigger.id },
        "deploymentTriggerDelete",
      );
      console.log(`  ${service.name}: removed stale deploy trigger for ${trigger.branch}`);
    } catch (error) {
      console.warn(`  ${service.name}: could not remove stale deploy trigger for ${trigger.branch} (${error.message})`);
    }
  }

  try {
    const created = await gql(
      `mutation deploymentTriggerCreate($input: DeploymentTriggerCreateInput!) {
        deploymentTriggerCreate(input: $input) { id branch repository provider }
      }`,
      {
        input: {
          projectId: project.id,
          environmentId: railwayEnv.id,
          serviceId: service.id,
          repository: REPO,
          branch,
          provider: "GITHUB",
        },
      },
      "deploymentTriggerCreate",
    );
    console.log(`  ${service.name}: deploy trigger created for ${branch}`);
    return created;
  } catch (error) {
    console.warn(`  ${service.name}: could not create deploy trigger for ${branch} (${error.message}); continuing`);
    return null;
  }
}

async function listDeploymentTriggers(projectId, environmentId, serviceId) {
  const data = await gql(
    `query deploymentTriggers($projectId: String!, $environmentId: String!, $serviceId: String!) {
      deploymentTriggers(projectId: $projectId, environmentId: $environmentId, serviceId: $serviceId) {
        edges { node { id projectId environmentId serviceId repository branch provider } }
      }
    }`,
    { projectId, environmentId, serviceId },
  );
  return data.deploymentTriggers.edges.map((edge) => edge.node);
}

async function updateServiceInstance(service, railwayEnv, spec) {
  const input = Object.fromEntries(Object.entries(spec.config).filter(([, value]) => value !== undefined && value !== null && value !== ""));
  try {
    await gql(
      `mutation serviceInstanceUpdate($serviceId: String!, $environmentId: String!, $input: ServiceInstanceUpdateInput!) {
        serviceInstanceUpdate(serviceId: $serviceId, environmentId: $environmentId, input: $input)
      }`,
      { serviceId: service.id, environmentId: railwayEnv.id, input },
      "serviceInstanceUpdate",
    );
    console.log(`  ${service.name}: service instance config updated`);
  } catch (error) {
    const message = error.message || String(error);
    if (message.includes("dockerfilePath") || message.includes("railwayConfigFile")) {
      const fallback = { ...input };
      delete fallback.dockerfilePath;
      delete fallback.railwayConfigFile;
      await gql(
        `mutation serviceInstanceUpdate($serviceId: String!, $environmentId: String!, $input: ServiceInstanceUpdateInput!) {
          serviceInstanceUpdate(serviceId: $serviceId, environmentId: $environmentId, input: $input)
        }`,
        { serviceId: service.id, environmentId: railwayEnv.id, input: fallback },
        "serviceInstanceUpdate",
      );
      console.log(`  ${service.name}: service instance config updated without file-path fields; Railway config-as-code remains source for build config`);
      return;
    }
    throw error;
  }
}

async function ensurePostgisVolume(project, railwayEnv, service, spec, envSpec) {
  const volumes = await listVolumes(project.id);
  const existing = volumes.find((volume) => volume.environmentId === railwayEnv.id && volume.serviceId === service.id);
  if (existing) {
    console.log(`  postgis: volume exists (${existing.id})`);
    return existing;
  }
  console.log(`  postgis: creating volume`);
  return gql(
    `mutation volumeCreate($input: VolumeCreateInput!) {
      volumeCreate(input: $input) { id name }
    }`,
    {
      input: {
        projectId: project.id,
        environmentId: railwayEnv.id,
        serviceId: service.id,
        mountPath: spec.volume.mountPath,
      },
    },
    "volumeCreate",
  );
}

async function listVolumes(projectId) {
  const data = await gql(
    `query project($id: String!) {
      project(id: $id) {
        volumes {
          edges {
            node {
              id
              name
              volumeInstances {
                edges { node { id environmentId serviceId mountPath } }
              }
            }
          }
        }
      }
    }`,
    { id: projectId },
  );
  return data.project.volumes.edges.flatMap((edge) => {
    const volume = edge.node;
    const instances = volume.volumeInstances?.edges?.map((instanceEdge) => instanceEdge.node) || [];
    return instances.map((instance) => ({ ...volume, ...instance, volumeId: volume.id, volumeName: volume.name }));
  });
}

async function ensureDomains(project, railwayEnv, services) {
  const result = new Map();
  for (const spec of serviceSpecs.filter((service) => service.publicDomain)) {
    const service = services.get(spec.name);
    const existing = await listDomains(project.id, railwayEnv.id, service.id);
    let domain = existing.serviceDomains?.[0]?.domain || null;
    if (!domain) {
      const configuredDomain = configuredRailwayDomain(railwayEnv.name, spec.name);
      if (configuredDomain) {
        domain = configuredDomain;
      }
    }
    if (!domain) {
      console.log(`  ${spec.name}: creating Railway service domain`);
      try {
        const created = await gql(
          `mutation serviceDomainCreate($input: ServiceDomainCreateInput!) {
            serviceDomainCreate(input: $input) { id domain }
          }`,
          { input: { serviceId: service.id, environmentId: railwayEnv.id, targetPort: spec.targetPort } },
          "serviceDomainCreate",
        );
        domain = created.domain;
      } catch (error) {
        console.warn(`  ${spec.name}: could not create Railway service domain (${error.message}); continuing without generated domain`);
      }
    }
    if (domain) {
      console.log(`  ${spec.name}: public domain ${domain}`);
      result.set(spec.name, domain.startsWith("http") ? domain : `https://${domain}`);
    }

    const customDomain = customDomainFor(railwayEnv.name, spec.name);
    if (customDomain && !existing.customDomains?.some((entry) => entry.domain === customDomain)) {
      console.log(`  ${spec.name}: adding custom domain ${customDomain}`);
      await gql(
        `mutation customDomainCreate($input: CustomDomainCreateInput!) {
          customDomainCreate(input: $input) { id domain }
        }`,
        {
          input: {
            projectId: project.id,
            environmentId: railwayEnv.id,
            serviceId: service.id,
            domain: customDomain,
            targetPort: spec.targetPort,
          },
        },
        "customDomainCreate",
      );
      result.set(spec.name, `https://${customDomain}`);
    }
  }
  return result;
}

async function listDomains(projectId, environmentId, serviceId) {
  const data = await gql(
    `query domains($projectId: String!, $environmentId: String!, $serviceId: String!) {
      domains(projectId: $projectId, environmentId: $environmentId, serviceId: $serviceId) {
        serviceDomains { id domain targetPort }
        customDomains { id domain }
      }
    }`,
    { projectId, environmentId, serviceId },
  );
  return data.domains;
}

async function upsertEnvironmentVariables(project, railwayEnv, envSpec, domains, services) {
  const varsByService = buildVariables(envSpec, domains);
  for (const [serviceName, variables] of Object.entries(varsByService)) {
    if (!serviceNameSet.has(serviceName)) continue;
    const service = services.get(serviceName);
    const cleanVariables = Object.fromEntries(Object.entries(variables).filter(([, value]) => value !== undefined && value !== null && String(value) !== ""));
    if (Object.keys(cleanVariables).length === 0) continue;
    await gql(
      `mutation variableCollectionUpsert($input: VariableCollectionUpsertInput!) {
        variableCollectionUpsert(input: $input)
      }`,
      {
        input: {
          projectId: project.id,
          environmentId: railwayEnv.id,
          serviceId: service.id,
          variables: cleanVariables,
          replace: false,
          skipDeploys: true,
        },
      },
      "variableCollectionUpsert",
    );
    console.log(`  ${serviceName}: upserted ${Object.keys(cleanVariables).length} variables`);
    await sleep(2_000);
  }
}

function buildVariables(envSpec, domains) {
  const suffix = envSpec.suffix;
  const appEnv = envSpec.appEnv;
  const postgisPassword = requiredEnv(`POSTGIS_PASSWORD_${suffix}`, "POSTGIS_PASSWORD");
  const databaseUrl = `postgres://wadd:${encodeURIComponent(postgisPassword)}@\${{postgis.RAILWAY_PRIVATE_DOMAIN}}:5432/wa_dd?sslmode=disable`;
  const internalApiToken = requiredEnv(`WADD_INTERNAL_API_TOKEN_${suffix}`, "WADD_INTERNAL_API_TOKEN");
  const webPublicUrl = env(`${suffix}_WEB_PUBLIC_URL`) || domains.get("web") || envSpec.siteUrlDefault;
  const apiPublicUrl = env(`${suffix}_API_PUBLIC_URL`) || domains.get("api") || "";
  const cloudflareAccountId = env(`${suffix}_CLOUDFLARE_ACCOUNT_ID`) || env("CLOUDFLARE_ACCOUNT_ID");
  const r2Endpoint = cloudflareAccountId ? `https://${cloudflareAccountId}.r2.cloudflarestorage.com` : "";

  return {
    postgis: {
      PGDATA: "/var/lib/postgresql/data/pgdata",
      POSTGRES_USER: "wadd",
      POSTGRES_DB: "wa_dd",
      POSTGRES_PASSWORD: postgisPassword,
    },
    api: {
      APP_ENV: appEnv,
      LOG_LEVEL: env(`${suffix}_LOG_LEVEL`) || env("LOG_LEVEL") || "info",
      DATABASE_URL: databaseUrl,
      SOURCE_USER_AGENT: env("SOURCE_USER_AGENT") || "wa-dd/0.0.1 (https://github.com/NolanJMcCafferty/wa-digital-democracy; contact: nolan-mccafferty)",
      WADD_INTERNAL_API_TOKEN: internalApiToken,
      CLERK_JWT_ISSUER: requiredEnv(`CLERK_JWT_ISSUER_${suffix}`, "CLERK_JWT_ISSUER"),
      CLERK_JWKS_URL: env(`CLERK_JWKS_URL_${suffix}`) || env("CLERK_JWKS_URL"),
      WADD_ADMIN_JWT_AUDIENCE: env(`${suffix}_WADD_ADMIN_JWT_AUDIENCE`) || env("WADD_ADMIN_JWT_AUDIENCE") || "wa-dd-admin",
    },
    web: {
      WADD_API_URL: "http://${{api.RAILWAY_PRIVATE_DOMAIN}}:8080",
      API_BASE_URL: "http://${{api.RAILWAY_PRIVATE_DOMAIN}}:8080",
      NEXT_PUBLIC_SITE_URL: webPublicUrl,
      WADD_INTERNAL_API_TOKEN: internalApiToken,
      NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY: requiredEnv(`NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY_${suffix}`, "NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY"),
      CLERK_SECRET_KEY: requiredEnv(`CLERK_SECRET_KEY_${suffix}`, "CLERK_SECRET_KEY"),
    },
    migrate: {
      DATABASE_URL: databaseUrl,
    },
    daily: {
      APP_ENV: appEnv,
      LOG_LEVEL: env(`${suffix}_LOG_LEVEL`) || env("LOG_LEVEL") || "info",
      DATABASE_URL: databaseUrl,
      OBJECT_STORE: env(`${suffix}_OBJECT_STORE`) || env("OBJECT_STORE") || "s3",
      S3_ENDPOINT_URL: env(`${suffix}_S3_ENDPOINT_URL`) || env("S3_ENDPOINT_URL") || r2Endpoint,
      S3_REGION: env(`${suffix}_S3_REGION`) || env("S3_REGION") || "auto",
      S3_BUCKET_RAW: env(`${suffix}_S3_BUCKET_RAW`) || env("S3_BUCKET_RAW") || envSpec.bucketDefault,
      S3_ACCESS_KEY_ID: requiredEnv(`S3_ACCESS_KEY_ID_${suffix}`, "S3_ACCESS_KEY_ID", "R2_ACCESS_KEY_ID"),
      S3_SECRET_ACCESS_KEY: requiredEnv(`S3_SECRET_ACCESS_KEY_${suffix}`, "S3_SECRET_ACCESS_KEY", "R2_SECRET_ACCESS_KEY"),
      S3_FORCE_PATH_STYLE: env(`${suffix}_S3_FORCE_PATH_STYLE`) || env("S3_FORCE_PATH_STYLE") || "false",
      S3_PREFIX: env(`${suffix}_S3_PREFIX`) || env("S3_PREFIX") || "raw",
      INVINTUS_EMBEDDER_KEY: requiredEnv(`INVINTUS_EMBEDDER_KEY_${suffix}`, "INVINTUS_EMBEDDER_KEY"),
      SOCRATA_APP_TOKEN: env(`SOCRATA_APP_TOKEN_${suffix}`) || env("SOCRATA_APP_TOKEN"),
      SOURCE_USER_AGENT: env("SOURCE_USER_AGENT") || "wa-dd/0.0.1 (https://github.com/NolanJMcCafferty/wa-digital-democracy; contact: nolan-mccafferty)",
      BIENNIUM: env(`${suffix}_DAILY_BIENNIUM`) || env("DAILY_BIENNIUM") || "2025-26",
    },
  };
}

async function deployEnvironment(railwayEnv, services) {
  for (const name of ["postgis", "migrate", "api", "web", "daily"]) {
    const service = services.get(name);
    try {
      const deploymentId = await gql(
        `mutation serviceInstanceDeployV2($serviceId: String!, $environmentId: String!, $commitSha: String) {
          serviceInstanceDeployV2(serviceId: $serviceId, environmentId: $environmentId, commitSha: $commitSha)
        }`,
        { serviceId: service.id, environmentId: railwayEnv.id, commitSha: deployCommitSha || null },
        "serviceInstanceDeployV2",
      );
      console.log(`  ${name}: deployment triggered (${deploymentId})`);
    } catch (error) {
      console.warn(`  ${name}: explicit deploy skipped (${error.message}); GitHub branch trigger will deploy on push`);
    }
  }
}

function customDomainFor(environmentName, serviceName) {
  const envPrefix = environmentName.toUpperCase();
  const servicePrefix = serviceName.toUpperCase();
  return env(`${envPrefix}_${servicePrefix}_CUSTOM_DOMAIN`) || env(`${servicePrefix}_CUSTOM_DOMAIN`) || "";
}

function configuredRailwayDomain(environmentName, serviceName) {
  const envPrefix = environmentName.toUpperCase();
  const servicePrefix = serviceName.toUpperCase();
  const direct = env(`${envPrefix}_${servicePrefix}_RAILWAY_DOMAIN`) || env(`${servicePrefix}_RAILWAY_DOMAIN`);
  if (direct) return direct;
  const subdomain = env(`${envPrefix}_${servicePrefix}_RAILWAY_SUBDOMAIN`) || env(`${servicePrefix}_RAILWAY_SUBDOMAIN`);
  if (!subdomain) return "";
  return subdomain.includes(".") ? subdomain : `${subdomain}.up.railway.app`;
}

async function gql(query, variables = {}, pick = null) {
  const attempts = 5;
  let lastError;
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      const response = await fetch(API_URL, {
        method: "POST",
        headers: {
          "Authorization": `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ query, variables }),
      });
      const text = await response.text();
      let json;
      try {
        json = JSON.parse(text);
      } catch {
        const error = new Error(`Railway API returned non-JSON HTTP ${response.status}: ${text.slice(0, 500)}`);
        error.retryable = response.status === 429 || response.status >= 500;
        throw error;
      }
      if (!response.ok || json.errors?.length) {
        const messages = json.errors?.map((error) => error.message).join("; ") || text;
        const error = new Error(`Railway API error: ${messages}`);
        error.retryable = response.status === 429 || response.status >= 500 || /timeout|timed out|rate|temporar|try again/i.test(messages);
        throw error;
      }
      return pick ? json.data[pick] : json.data;
    } catch (error) {
      lastError = error;
      if (!error.retryable || attempt === attempts) break;
      const delayMs = Math.min(45_000, 2_000 * 2 ** (attempt - 1));
      console.warn(`Railway API call failed (${error.message}); retrying in ${delayMs / 1000}s [${attempt}/${attempts}]`);
      await sleep(delayMs);
    }
  }
  throw lastError;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function env(name) {
  return process.env[name];
}

function requiredEnv(...names) {
  for (const name of names) {
    const value = env(name);
    if (value && value.trim() !== "") return value;
  }
  throw new Error(`Missing required environment variable: one of ${names.join(", ")}`);
}

function emptyToNull(value) {
  return value && value.trim() !== "" ? value : null;
}

function fail(message) {
  console.error(message);
  process.exit(1);
}

function printHelp() {
  console.log(`Usage:
  RAILWAY_API_TOKEN=... scripts/bootstrap-railway.mjs apply [--deploy]
  RAILWAY_API_TOKEN=... scripts/bootstrap-railway.mjs deploy
  scripts/bootstrap-railway.mjs apply --dry-run

Creates/reconciles one Railway project with production and staging environments.
Production deploys from main; staging deploys from staging.

Required secrets can be environment-specific, falling back to shared names:
  POSTGIS_PASSWORD[_PRODUCTION|_STAGING]
  WADD_INTERNAL_API_TOKEN[_PRODUCTION|_STAGING]
  INVINTUS_EMBEDDER_KEY[_PRODUCTION|_STAGING]
  CLERK_SECRET_KEY[_PRODUCTION|_STAGING]
  NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY[_PRODUCTION|_STAGING]
  CLERK_JWT_ISSUER[_PRODUCTION|_STAGING]
  S3_ACCESS_KEY_ID[_PRODUCTION|_STAGING] or R2_ACCESS_KEY_ID
  S3_SECRET_ACCESS_KEY[_PRODUCTION|_STAGING] or R2_SECRET_ACCESS_KEY

Optional:
  RAILWAY_WORKSPACE_ID
  PRODUCTION_WEB_PUBLIC_URL / STAGING_WEB_PUBLIC_URL
  PRODUCTION_API_PUBLIC_URL / STAGING_API_PUBLIC_URL
  PRODUCTION_WEB_CUSTOM_DOMAIN / STAGING_WEB_CUSTOM_DOMAIN
  PRODUCTION_API_CUSTOM_DOMAIN / STAGING_API_CUSTOM_DOMAIN
  CLOUDFLARE_ACCOUNT_ID
`);
}

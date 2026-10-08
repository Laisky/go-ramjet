# llm-storm Deep Research retirement

The custom `deep-research` chat choice backed by llm-storm is retired. It is no longer offered in the model selector and the client no longer creates or polls research jobs. Saved configurations and shared URLs selecting this retired choice automatically use the default chat model (or the default image model for an invalid draw-model selection); other settings and history are preserved. Ordinary chat, Responses, MCP tools, browser/search/retrieval, embeddings, image generation, and upstream provider models such as `o3-deep-research` are separate features.

For cached clients, `POST /gptchat/deepresearch` and `GET /gptchat/deepresearch/:task_id` return HTTP 410 with code `deep_research_retired`. These static compatibility routes bypass the active API Redis rate limiter. Neither handler authenticates, enqueues, reads task data, or contacts the worker. The compatibility response contains no user data. The status endpoint no longer retrieves unmaterialized worker results.

## Existing data

Completed research answers already saved in browser chat history remain readable and exportable with their original messages and references. Retirement does not migrate, erase, or rewrite these messages. Pending browser records are retained but cannot finish or poll through the retired API; no replacement answer is synthesized.

Worker records are separate Redis data: `laisky/tasks/llm_storm/pending`, `laisky/tasks/llm_storm/result/{id}`, and Co-STORM session keys. The worker previously assigned results a 30-day TTL; that existing expiry policy is unchanged. Retirement must not flush Redis, delete session keys, remove volumes, or discard history. Operators can inspect existing records using authorized operational access, without exposing prompts or API keys. No indefinite archival guarantee is introduced.

## Deployment and shutdown order

1. Verify all actual containers and callers, including GraphQL enqueue and direct llm-storm research/conversation endpoints. Check queue depth, active jobs/sessions, service labels, mounts, restart policies, and automatic deploy/start definitions. A second service definition or other active caller requires explicit resolution before stopping the worker.
2. Merge only after the standard reviewed workflow and exact-head checks pass. The go-ramjet master push workflow builds an immutable short-SHA image and automatically deploys its configured b1 instance. Confirm its actual running image and HTTP 410 behavior; update any other running go-ramjet caller explicitly before proceeding.
3. Retire only the identified llm-storm service definition and automatic start policy after no approved caller or active job depends on it. Stop only that container. Preserve its image, stopped container where practical, Redis records, env files, volumes, and unrelated services.
4. Confirm the container is stopped and remains stopped, normal go-ramjet health and synthetic ordinary flows pass, and the worker/host CPU measurement decreases.

## Reversible recovery

Record the last image digest, container/service name, Compose file and Git commit, restart policy, and any mounts before stopping. The reviewed code and deployment commits provide recovery points without archive branches. To restore within an authorized recovery, revert the retirement commits, use the recorded image/Compose service configuration, restore its prior restart policy, and start only that service after checking callers and data retention. Do not recreate or remove an entire Compose project. Expired Redis results cannot be restored merely by starting the worker.

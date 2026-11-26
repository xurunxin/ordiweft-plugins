# Higgsfield for Ordiweft

Higgsfield is an image and video provider delivered as a trusted Ordiweft SDK v2 process plugin. The catalog snapshot dated 2026-11-27 has 106 distinct generation endpoints and two Marketing Studio workflow presets. In the canvas, use the shared image and video generation nodes and select the Higgsfield provider and model.

## Install

Use a Linux amd64 or Windows amd64 host that supports the `higgsfield-stdio-v1` profile. In Ordiweft's plugin center refresh the official npm catalog, install and activate the reviewed `ordiweft-migrations/higgsfield` release distributed as `@daoxialiuren/ordiweft-higgsfield`. The host selects the executable by platform from the package stored under its verified release digest. The executable runs with the server's OS identity, so a new release requires a new approved digest in the host.

If a legacy Higgsfield process is already configured in `data_dir/plugins`, drain its original tasks and remove that provisioning before activating the catalog release.

Inject `HF_CREDENTIALS` into the Ordiweft server's process environment as the vendor-issued `KEY_ID:KEY_SECRET`. In **渠道与模型**, select Higgsfield, map the plugin's `API_KEY` to the environment variable `HF_CREDENTIALS`, and enable the provider. Configure the variable in the server's launcher or service manager if its value lives in a local `.env` file. Keep the value out of `plugin.json` and logs.

The package uses the host's trusted process loader and needs HTTPS access to Higgsfield and its media storage. It runs with the server's OS identity. The host stdio limit is 64 MiB including Base64 encoding and JSON framing. This limits combined raw references to less than 48 MiB per job, subject to the host's upload limit. The upstream presigned file upload documents JPEG, PNG, WebP, GIF, MP4 and WAV for local canvas assets.

## MAGI-2

Choose a mode by model label. All three modes support 4–30 seconds, 1080p, six aspect ratios, optional generated audio and a seed from 0 to 4294967295. The manifest puts the three endpoints in a shared concurrency bucket of one. The documented list price is USD 0.08 per requested second.

| Mode | Prompt and inputs |
| --- | --- |
| [Text to Video](https://open.higgsfield.ai/models/sand/magi-2/text-to-video/api-reference) | Required prompt, zero references |
| [Image to Video](https://open.higgsfield.ai/models/sand/magi-2/image-to-video/api-reference) | Required first image, optional last image and prompt |
| [Reference to Video](https://open.higgsfield.ai/models/sand/magi-2/reference-to-video/api-reference) | At least one reference across up to 10 images, 5 videos and 5 audio clips, optional prompt |

The adapter obtains presigned upload grants, sends original bytes to the designated storage endpoint, then submits the generation once. The request ID is checkpointed before polling. Polling only queries the original ID. For an interrupted job with a checkpoint, the documented status API can be called with that same ID from the server. The trusted stdio runtime has no Ordiweft automatic recovery across restart.

## Catalog and build

[Catalog source snapshot](catalog.json) records exact endpoint IDs, model titles, modes and request schemas derived from the public [Explore](https://open.higgsfield.ai/explore), [Pricing](https://open.higgsfield.ai/pricing) and linked model API pages. The official [OpenAPI](https://docs.higgsfield.ai/docs/openapi.json) supplemented older models. The [API docs](https://docs.higgsfield.ai/docs) and [official SDK](https://github.com/higgsfield-ai/higgsfield-js/tree/d4d2d48094079ee2e17206e848151e27372bb3dd) define the shared lifecycle. Public documentation describes schema coverage. Your API organization controls which endpoints a key can run.

The two Marketing Studio presets share `marketing-studio/image`. The Seedance 2.5 finalize-draft operation requires a provider-specific binding from an owned canvas result to its upstream draft ID. This operation was left out of the process package. Provider-side validators enforce additional media length and account restrictions.

With Go 1.27.0 and a local export of the Ordiweft SDK from host commit `b9c5abce0f27b19231030f9d3b5ea48729f7b96f`, place the SDK at `./sdk` in an isolated copy of the project, or use a separate Go modfile with a local replace. The JSON Schema dependency is the same version pinned by that host. Use an already populated Go module cache for an offline build.

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
rtk proxy go test ./...
rtk proxy go build -trimpath -buildvcs=false -ldflags=-buildid= -o <output>/higgsfield/provider.exe .
```

In an isolated development directory run the built executable with the `package` argument and save its `manifest` member as a UTF-8 JSON file. Pass this reviewed manifest and the public SDK export to `scripts/build-higgsfield-stdio.ps1`. The builder produces both binaries and their inventory in a new output directory without running them. Before using `ordiweft-plugin pack`, verify that each built adapter emits the supplied manifest.

## Verification

The offline tests exercise MAGI-2 payloads, named first and last frames, mixed image/video/audio references, signed uploads with no API key on storage requests, checkpoint-before-poll, and refusal without a second submission. The 2026-11-27 live test used one 4-second submission per mode. Higgsfield accepted the upload authorization and fixture uploads, then returned HTTP 403 `model_unavailable` for all three generation endpoints. There were no accepted request IDs or output videos.

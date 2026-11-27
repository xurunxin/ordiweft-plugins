import {readFile} from 'node:fs/promises'

const {ACTIONS_ID_TOKEN_REQUEST_URL: requestURL, ACTIONS_ID_TOKEN_REQUEST_TOKEN: requestToken} = process.env
if (!requestURL || !requestToken) throw new Error('GitHub Actions OIDC is unavailable. Check id-token: write on the stage job.')

const {name} = JSON.parse(await readFile(process.argv[2], 'utf8'))
const identityURL = new URL(requestURL)
identityURL.searchParams.set('audience', 'npm:registry.npmjs.org')
const identity = await fetch(identityURL, {
  headers: {Accept: 'application/json', Authorization: `Bearer ${requestToken}`},
  redirect: 'error',
  signal: AbortSignal.timeout(15_000)
})
if (!identity.ok) throw new Error(`GitHub OIDC token request returned HTTP ${identity.status}.`)

const {value: idToken} = await identity.json()
if (typeof idToken !== 'string' || !idToken) throw new Error('GitHub OIDC response did not include a token.')

const exchangeURL = `https://registry.npmjs.org/-/npm/v1/oidc/token/exchange/package/${encodeURIComponent(name)}`
const exchange = await fetch(exchangeURL, {
  method: 'POST',
  headers: {Authorization: `Bearer ${idToken}`},
  redirect: 'error',
  signal: AbortSignal.timeout(15_000)
})
if (!exchange.ok) throw new Error(`npm OIDC exchange for ${name} returned HTTP ${exchange.status}.`)

const {token} = await exchange.json()
if (typeof token !== 'string' || !token) throw new Error('npm OIDC exchange response did not include a token.')

console.log(`npm OIDC authentication verified for ${name} (HTTP ${exchange.status}).`)

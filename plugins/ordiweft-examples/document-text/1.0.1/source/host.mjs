// The host/runtime supplies exchange. This module grants no network or task
// authority and never invents a replacement ID after an uncertain response.
export const version = 1;
export const maxControlBytes = 128 * 1024;
const services = { connections: ['request'], assets: ['describe', 'read', 'decode', 'release'], transfers: ['upload', 'download'], jobs: ['checkpoint', 'query', 'progress', 'wait'], outputs: ['stage'], diagnostics: ['report'], usage: ['report'], media: ['inspect', 'derive'], state: ['get', 'compareAndSet'], catalog: ['list', 'search'] };
const opaque = /^[A-Za-z0-9_-]{1,128}$/;
const code = /^[a-z][a-z0-9_]{0,63}$/;
const validCode = v => typeof v === 'string' && code.test(v);
const object = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const json = value => {
    const text = JSON.stringify(value);
    if (typeof text !== 'string' || Buffer.byteLength(text) > maxControlBytes)
        throw new TypeError('plugin_contract_invalid');
    return JSON.parse(text);
};
const fail = () => { throw new TypeError('plugin_contract_invalid'); };
const failure = v => object(v) && validCode(v.code) && ['prepare', 'submit', 'observe', 'output', 'adopt'].includes(v.stage) && ['not_sent', 'sent', 'unknown'].includes(v.submission) && (v.requestId === undefined || typeof v.requestId === 'string' && Buffer.byteLength(v.requestId) <= 256 && !/[\0\r\n]/.test(v.requestId)) && Object.keys(v).every(k => ['code', 'stage', 'submission', 'requestId'].includes(k));
export class HostError extends Error {
    constructor(value) {
        if (!failure(value))
            fail();
        super('plugin_host_' + value.code);
        this.name = 'HostError';
        this.failure = Object.freeze({ ...value });
    }
}
export function createClient(exchange) {
    if (typeof exchange !== 'function')
        fail();
    async function call(id, service, operation, input, { signal } = {}) {
        if (typeof id !== 'string' || !opaque.test(id) || typeof service !== 'string' || !Object.hasOwn(services, service) || !services[service].includes(operation) || !object(input))
            fail();
        signal?.throwIfAborted();
        const request = json({ version, id, service, operation, input });
        const raw = await exchange(request, { signal });
        const reply = json(raw);
        if (!object(reply) || reply.version !== version || reply.id !== id || Object.keys(reply).some(k => !['version', 'id', 'result', 'error'].includes(k)))
            fail();
        if ('error' in reply) {
            if ('result' in reply || !failure(reply.error))
                fail();
            throw new HostError(reply.error);
        }
        if (!object(reply.result))
            fail();
        return reply.result;
    }
    return Object.freeze({ call,
        async reportUsage(id, meters, options) {
            if (!Array.isArray(meters) || meters.length === 0 || meters.length > 64)
                fail();
            for (const m of meters)
                if (!object(m) || !validCode(m.name) || !validCode(m.unit) || typeof m.quantity !== 'string' || Buffer.byteLength(m.quantity) > 80 || !/^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$/.test(m.quantity) || !['provider_reported', 'estimated'].includes(m.source) || Object.keys(m).some(k => !['name', 'unit', 'quantity', 'source'].includes(k)))
                    fail();
            return call(id, 'usage', 'report', { meters }, options);
        },
        async reportDiagnostic(id, report, options) {
            if (!failure(report)) {
                fail();
            }
            return call(id, 'diagnostics', 'report', { failure: report }, options);
        },
    });
}
// The runtime supplies these private streams. No socket, URL, key or reconnect
// policy is created here. A caller must serialize operations on this channel.
export function createStreamClient(readable, writable) {
    if (typeof readable?.[Symbol.asyncIterator] !== 'function')
        fail();
    return streamClient(readable, writable, readable[Symbol.asyncIterator]());
}
async function readFrame(iterator) {
    let bytes = Buffer.alloc(0);
    for (;;) {
        const { value, done } = await iterator.next();
        if (done) {
            throw new Error('plugin_channel_response_lost');
        }
        const chunk = Buffer.isBuffer(value) ? value : Buffer.from(value);
        if (bytes.length + chunk.length > maxControlBytes + 1) {
            fail();
        }
        bytes = Buffer.concat([bytes, chunk]);
        const newline = bytes.indexOf(10);
        if (newline < 0) {
            continue;
        }
        if (newline !== bytes.length - 1 || newline > maxControlBytes) {
            fail();
        }
        return JSON.parse(bytes.subarray(0, newline).toString('utf8'));
    }
}
function streamClient(readable, writable, iterator) {
    if (typeof readable?.[Symbol.asyncIterator] !== 'function' || typeof readable.destroy !== 'function' || typeof writable?.write !== 'function' || typeof writable.destroy !== 'function')
        fail();
    let busy = false, terminal;
    const close = (reason = new Error('plugin_channel_closed')) => {
        terminal ??= reason;
        readable.destroy();
        writable.destroy();
    };
    readable.on('error', close);
    writable.on('error', close);
    const client = createClient(async (request, { signal }) => {
        if (terminal)
            throw terminal;
        if (busy)
            throw new Error('plugin_channel_busy');
        busy = true;
        const abort = () => close(signal.reason ?? new Error('plugin_channel_aborted'));
        signal?.addEventListener('abort', abort, { once: true });
        try {
            signal?.throwIfAborted();
            const body = Buffer.from(JSON.stringify(request) + '\n');
            await new Promise((resolve, reject) => writable.write(body, error => error ? reject(error) : resolve()));
            return await readFrame(iterator);
        }
        catch (error) {
            close(error);
            throw terminal;
        }
        finally {
            busy = false;
            signal?.removeEventListener('abort', abort);
        }
    });
    const protect = fn => async (...args) => {
        try {
            return await fn(...args);
        }
        catch (error) {
            if (error instanceof HostError) {
                throw error;
            }
            close(error);
            throw terminal;
        }
    };
    return Object.freeze({ call: protect(client.call), reportUsage: protect(client.reportUsage), reportDiagnostic: protect(client.reportDiagnostic), close });
}
const validHandle = value => object(value) && typeof value.id === 'string' && opaque.test(value.id) && typeof value.sha256 === 'string' && /^[a-f0-9]{64}$/.test(value.sha256) && Number.isSafeInteger(value.bytes) && value.bytes >= 0 && value.bytes <= 512 * 1024 * 1024 && typeof value.mime === 'string' && value.mime.length <= 128 && /^[a-z0-9!#$&^_.+-]+\/[a-z0-9!#$&^_.+-]+$/.test(value.mime) && Object.keys(value).every(key => ['id', 'sha256', 'bytes', 'mime'].includes(key));
// Reads the runtime's fixed start frame. Credentials and raw input bytes never
// appear in this mode; input handles are consumed through the original host.
export function openTask(readable, writable, options = {}) {
    return openInvocation(readable, writable, options, start => {
        if (!object(start) || start.hostProtocol !== 1 || ![1, 2].includes(start.apiVersion) || !object(start.generation) || Object.keys(start).some(key => !['hostProtocol', 'apiVersion', 'generation', 'inputs'].includes(key)) || !Array.isArray(start.inputs) || start.inputs.length > 80)
            fail();
        for (const input of start.inputs) {
            if (!object(input) || !['image', 'video', 'audio', 'voice'].includes(input.kind) || typeof input.role !== 'string' || Buffer.byteLength(input.role) > 128 || /[\0\r\n]/.test(input.role) || !validHandle(input.asset) || Object.keys(input).some(key => !['kind', 'role', 'asset'].includes(key)))
                fail();
        }
        return {generation:start.generation,inputs:start.inputs};
    }, (outputs,metadata) => {
        if (!validOutputs(outputs) || metadata !== undefined && (!object(metadata) || Buffer.byteLength(JSON.stringify(metadata)) > 65536))
            fail();
        return {hostProtocol:1,kind:'complete',outputs,...(metadata===undefined?{}:{metadata})};
    });
}

const validOutputs = outputs => Array.isArray(outputs) && outputs.length <= 100 && new Set(outputs).size === outputs.length && outputs.every(id => typeof id === 'string' && opaque.test(id));

export function openAction(readable,writable,options={}) {
    return openInvocation(readable,writable,options,start => {
        if (!object(start) || start.hostProtocol !== 1 || start.kind !== 'invoke' || typeof start.portId !== 'string' || !/^[a-z][a-z0-9-]{0,63}$/.test(start.portId) || !['processor','worker'].includes(start.portKind) || !object(start.arguments) || Buffer.byteLength(JSON.stringify(start.arguments)) > 65536 || !Array.isArray(start.inputs) || start.inputs.length > 80 || Object.keys(start).some(key=>!['hostProtocol','kind','portId','portKind','arguments','inputs'].includes(key)))
            fail();
        let bytes=0;
        for(const input of start.inputs) {
            if(!object(input)||!['image','video','audio','voice','script','document','file','text','json'].includes(input.kind)||typeof input.role!=='string'||Buffer.byteLength(input.role)>128||/[\0\r\n]/.test(input.role)||!validHandle(input.asset)||Object.keys(input).some(key=>!['kind','role','asset'].includes(key)))
                fail();
            bytes+=input.asset.bytes;
            if(bytes>256*1024*1024)fail();
        }
        return {portId:start.portId,portKind:start.portKind,arguments:start.arguments,inputs:start.inputs};
    },(outputs,value)=>{
        if(!validOutputs(outputs)||!object(value)||Buffer.byteLength(JSON.stringify(value))>65536)fail();
        return {hostProtocol:1,kind:'action-complete',outputs,value};
    });
}

async function openInvocation(readable,writable,{signal},project,completion) {
    if (typeof readable?.[Symbol.asyncIterator] !== 'function')
        fail();
    const iterator = readable[Symbol.asyncIterator]();
    const host = streamClient(readable, writable, iterator);
    const abort = () => host.close(signal.reason ?? new Error('plugin_channel_aborted'));
    signal?.addEventListener('abort', abort, { once: true });
    try {
        signal?.throwIfAborted();
        const start = await readFrame(iterator);
        const fields=project(start);
        return Object.freeze({host,...fields,
            async complete(...args) {
                try {
                    const frame = json(completion(...args));
                    await new Promise((resolve, reject) => writable.write(JSON.stringify(frame) + '\n', error => error ? reject(error) : resolve()));
                }
                finally {
                    signal?.removeEventListener('abort', abort);
                    host.close();
                }
            },
        });
    }
    catch (error) {
        signal?.removeEventListener('abort', abort);
        host.close();
        throw error;
    }
}

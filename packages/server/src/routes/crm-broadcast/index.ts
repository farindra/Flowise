import express, { Request, Response } from 'express'
import http from 'http'
import https from 'https'

const router = express.Router()

const CRM_SERVICE_URL = process.env.CRM_SERVICE_URL || 'http://127.0.0.1:8083'
const CRM_INTERNAL_KEY = process.env.CRM_INTERNAL_KEY || 'ob-crm-internal-2026'

/**
 * IMPORTANT — this router is mounted at `/api/v1/crm-broadcast`, deliberately a
 * *sibling* of `/api/v1/crm` rather than a child of it.
 *
 * `/api/v1/crm/` is in WHITELIST_URLS (packages/server/src/utils/constants.ts),
 * and the auth middleware short-circuits whitelisted paths with a bare next().
 * The check is `req.path.startsWith(url)`, so `/api/v1/crm-broadcast/...` does
 * NOT match `/api/v1/crm/` and therefore still requires a valid session.
 *
 * Mounting this under `/crm/broadcasts` would silently inherit that bypass and
 * expose campaign launching to the public internet. Do not move it.
 */

function forwardQuery(req: Request, path: string): string {
    const qs = new URLSearchParams(req.query as Record<string, string>).toString()
    return qs ? `${path}?${qs}` : path
}

function proxy(method: string, pathFn: (req: Request) => string) {
    return (req: Request, res: Response) => {
        const url = `${CRM_SERVICE_URL}${pathFn(req)}`
        const lib = url.startsWith('https') ? https : http

        const options: http.RequestOptions = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'X-Internal-Key': CRM_INTERNAL_KEY
            }
        }

        const proxyReq = lib.request(url, options, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
            // Preserved so .xlsx template/export downloads keep their filename.
            if (proxyRes.headers['content-disposition']) {
                res.setHeader('Content-Disposition', proxyRes.headers['content-disposition'] as string)
            }
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'CRM service unreachable', detail: err.message })
        })

        if (method !== 'GET' && method !== 'DELETE') {
            const bodyStr = req.body !== undefined ? JSON.stringify(req.body) : ''
            proxyReq.setHeader('Content-Length', Buffer.byteLength(bodyStr))
            proxyReq.write(bodyStr)
        }
        proxyReq.end()
    }
}

/**
 * Multipart passthrough. The JSON proxy cannot be used for file uploads: the
 * body must stay an untouched stream so the multipart boundary survives.
 */
function proxyMultipart(pathFn: (req: Request) => string) {
    return (req: Request, res: Response) => {
        const url = `${CRM_SERVICE_URL}${pathFn(req)}`
        const lib = url.startsWith('https') ? https : http

        const headers: http.OutgoingHttpHeaders = {
            'X-Internal-Key': CRM_INTERNAL_KEY
        }
        if (req.headers['content-type']) headers['Content-Type'] = req.headers['content-type']
        if (req.headers['content-length']) headers['Content-Length'] = req.headers['content-length']

        const proxyReq = lib.request(url, { method: 'POST', headers }, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'CRM service unreachable', detail: err.message })
        })
        req.pipe(proxyReq)
    }
}

// ── Config / lookups ─────────────────────────────────────────────────────────
router.get(
    '/throttle-defaults',
    proxy('GET', () => '/api/broadcasts/throttle-defaults')
)
router.get(
    '/senders',
    proxy('GET', () => '/api/broadcasts/senders')
)
router.get(
    '/chat-sources',
    proxy('GET', () => '/api/broadcasts/chat-sources')
)
router.get(
    '/wilayah',
    proxy('GET', () => '/api/customers/wilayah')
)
router.get(
    '/template',
    proxy('GET', () => '/api/broadcasts/template')
)

// ── Opt-outs ─────────────────────────────────────────────────────────────────
router.get(
    '/optouts',
    proxy('GET', () => '/api/broadcasts/optouts')
)
router.post(
    '/optouts',
    proxy('POST', () => '/api/broadcasts/optouts')
)
router.delete(
    '/optouts/:phone',
    proxy('DELETE', (req) => `/api/broadcasts/optouts/${encodeURIComponent(req.params.phone)}`)
)

// ── Audience helpers ─────────────────────────────────────────────────────────
router.post(
    '/preview-audience',
    proxy('POST', () => '/api/broadcasts/preview-audience')
)
router.post(
    '/audience-file',
    proxyMultipart(() => '/api/broadcasts/audience-file')
)

// ── Media ────────────────────────────────────────────────────────────────────
router.post(
    '/media',
    proxyMultipart(() => '/api/broadcast-media')
)
router.get(
    '/media/:name',
    proxy('GET', (req) => `/api/broadcast-media/${encodeURIComponent(req.params.name)}`)
)

// ── Campaign CRUD ────────────────────────────────────────────────────────────
router.get(
    '/broadcasts',
    proxy('GET', (req) => forwardQuery(req, '/api/broadcasts'))
)
router.post(
    '/broadcasts',
    proxy('POST', () => '/api/broadcasts')
)
router.get(
    '/broadcasts/:id',
    proxy('GET', (req) => `/api/broadcasts/${req.params.id}`)
)
router.put(
    '/broadcasts/:id',
    proxy('PUT', (req) => `/api/broadcasts/${req.params.id}`)
)
router.delete(
    '/broadcasts/:id',
    proxy('DELETE', (req) => `/api/broadcasts/${req.params.id}`)
)

// ── Audience build & results ─────────────────────────────────────────────────
router.post(
    '/broadcasts/:id/audience',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/audience`)
)
router.get(
    '/broadcasts/:id/audience/:jobId',
    proxy('GET', (req) => `/api/broadcasts/${req.params.id}/audience/${req.params.jobId}`)
)
router.get(
    '/broadcasts/:id/recipients',
    proxy('GET', (req) => forwardQuery(req, `/api/broadcasts/${req.params.id}/recipients`))
)
router.get(
    '/broadcasts/:id/export',
    proxy('GET', (req) => `/api/broadcasts/${req.params.id}/export`)
)

// ── Controls ─────────────────────────────────────────────────────────────────
router.post(
    '/broadcasts/:id/test-send',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/test-send`)
)
router.post(
    '/broadcasts/:id/start',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/start`)
)
router.post(
    '/broadcasts/:id/pause',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/pause`)
)
router.post(
    '/broadcasts/:id/resume',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/resume`)
)
router.post(
    '/broadcasts/:id/cancel',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/cancel`)
)
router.post(
    '/broadcasts/:id/retry',
    proxy('POST', (req) => `/api/broadcasts/${req.params.id}/retry`)
)

export default router

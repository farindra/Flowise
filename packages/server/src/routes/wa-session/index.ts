import express, { Request, Response } from 'express'
import https from 'https'
import http from 'http'

const router = express.Router()

const WA_SERVICE_URL = process.env.WA_SERVICE_URL || 'http://127.0.0.1:8082'
const WA_INTERNAL_KEY = process.env.WA_INTERNAL_KEY || 'ob-wa-internal-2026'

function proxy(method: string, pathFn: (req: Request) => string) {
    return (req: Request, res: Response) => {
        const url = `${WA_SERVICE_URL}${pathFn(req)}`
        const lib = url.startsWith('https') ? https : http

        const options: http.RequestOptions = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'X-Internal-Key': WA_INTERNAL_KEY
            }
        }

        const proxyReq = lib.request(url, options, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'WA service unreachable', detail: err.message })
        })

        if (method !== 'GET' && method !== 'DELETE') {
            // Express json middleware already consumed the stream — serialize parsed body
            const bodyStr = req.body !== undefined ? JSON.stringify(req.body) : ''
            proxyReq.setHeader('Content-Length', Buffer.byteLength(bodyStr))
            proxyReq.write(bodyStr)
        }
        proxyReq.end()
    }
}

// Sessions CRUD
router.get(
    '/sessions',
    proxy('GET', () => '/api/sessions')
)
router.post(
    '/sessions',
    proxy('POST', () => '/api/sessions')
)
router.put(
    '/sessions/:id',
    proxy('PUT', (req) => `/api/sessions/${req.params.id}`)
)
router.delete(
    '/sessions/:id',
    proxy('DELETE', (req) => `/api/sessions/${req.params.id}`)
)

// Per-session control
router.get(
    '/sessions/:id/status',
    proxy('GET', (req) => `/api/sessions/${req.params.id}/status`)
)
router.get(
    '/sessions/:id/qr',
    proxy('GET', (req) => `/api/sessions/${req.params.id}/qr`)
)
router.post(
    '/sessions/:id/connect',
    proxy('POST', (req) => `/api/sessions/${req.params.id}/connect`)
)
router.post(
    '/sessions/:id/logout',
    proxy('POST', (req) => `/api/sessions/${req.params.id}/logout`)
)
router.post(
    '/sessions/:id/pair-phone',
    proxy('POST', (req) => {
        const phone = req.query.phone as string
        const path = `/api/sessions/${req.params.id}/pair-phone`
        return phone ? `${path}?phone=${encodeURIComponent(phone)}` : path
    })
)

export default router

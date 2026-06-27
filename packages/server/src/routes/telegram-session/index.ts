import express, { Request, Response } from 'express'
import https from 'https'
import http from 'http'

const router = express.Router()

const TG_SERVICE_URL = process.env.TG_SERVICE_URL || 'http://127.0.0.1:8081'
const TG_INTERNAL_KEY = process.env.TG_INTERNAL_KEY || 'ob-tg-internal-2026'

function proxyRequest(method: string, path: string) {
    return async (req: Request, res: Response) => {
        const url = `${TG_SERVICE_URL}${path}`
        const lib = url.startsWith('https') ? https : http

        const options: http.RequestOptions = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'X-Internal-Key': TG_INTERNAL_KEY
            }
        }

        const proxyReq = lib.request(url, options, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'Telegram service unreachable', detail: err.message })
        })

        if (method !== 'GET' && method !== 'DELETE') {
            req.pipe(proxyReq)
        } else {
            proxyReq.end()
        }
    }
}

function proxyWithParam(method: string, pathFn: (id: string) => string) {
    return async (req: Request, res: Response) => {
        const id = req.params.id
        const url = `${TG_SERVICE_URL}${pathFn(id)}`
        const lib = url.startsWith('https') ? https : http

        const options: http.RequestOptions = {
            method,
            headers: {
                'Content-Type': 'application/json',
                'X-Internal-Key': TG_INTERNAL_KEY
            }
        }

        const proxyReq = lib.request(url, options, (proxyRes) => {
            res.status(proxyRes.statusCode || 200)
            res.setHeader('Content-Type', proxyRes.headers['content-type'] || 'application/json')
            proxyRes.pipe(res)
        })
        proxyReq.on('error', (err) => {
            res.status(502).json({ error: 'Telegram service unreachable', detail: err.message })
        })

        if (method !== 'GET' && method !== 'DELETE') {
            req.pipe(proxyReq)
        } else {
            proxyReq.end()
        }
    }
}

// GET /api/v1/telegram-session/bots
router.get('/bots', proxyRequest('GET', '/api/bots'))

// POST /api/v1/telegram-session/bots
router.post('/bots', proxyRequest('POST', '/api/bots'))

// PUT /api/v1/telegram-session/bots/:id
router.put('/bots/:id', proxyWithParam('PUT', (id) => `/api/bots/${id}`))

// DELETE /api/v1/telegram-session/bots/:id
router.delete('/bots/:id', proxyWithParam('DELETE', (id) => `/api/bots/${id}`))

// POST /api/v1/telegram-session/bots/:id/register
router.post('/bots/:id/register', proxyWithParam('POST', (id) => `/api/bots/${id}/register`))

export default router

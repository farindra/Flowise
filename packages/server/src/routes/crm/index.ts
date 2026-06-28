import express from 'express'
import http from 'http'

const router = express.Router()

function proxyToCRM(path: string, method: string, req: express.Request, res: express.Response, body?: string) {
    const options = {
        hostname: '127.0.0.1',
        port: 8083,
        path,
        method,
        headers: {
            'X-Internal-Key': 'ob-crm-internal-2026',
            'Content-Type': 'application/json',
            ...(body ? { 'Content-Length': Buffer.byteLength(body) } : {})
        }
    }

    const proxyReq = http.request(options, (proxyRes) => {
        res.setHeader('Content-Type', 'application/json')
        res.status(proxyRes.statusCode || 200)
        proxyRes.pipe(res)
    })
    proxyReq.on('error', (e) => res.status(503).json({ error: 'CRM unavailable: ' + e.message }))
    if (body) proxyReq.write(body)
    proxyReq.end()
}

router.get('/leads', (req, res) => {
    const qs = new URLSearchParams(req.query as Record<string, string>).toString()
    proxyToCRM(`/api/leads${qs ? '?' + qs : ''}`, 'GET', req, res)
})

router.get('/leads/:id', (req, res) => {
    proxyToCRM(`/api/leads/${req.params.id}`, 'GET', req, res)
})

router.put('/leads/:id', (req, res) => {
    const body = JSON.stringify(req.body)
    proxyToCRM(`/api/leads/${req.params.id}`, 'PUT', req, res, body)
})

router.get('/stats', (req, res) => {
    const qs = new URLSearchParams(req.query as Record<string, string>).toString()
    proxyToCRM(`/api/stats${qs ? '?' + qs : ''}`, 'GET', req, res)
})

export default router

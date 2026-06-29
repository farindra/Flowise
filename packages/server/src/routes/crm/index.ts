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

router.get('/salesmen', (req, res) => proxyToCRM('/api/salesmen', 'GET', req, res))
router.post('/salesmen', (req, res) => {
    const body = JSON.stringify(req.body)
    proxyToCRM('/api/salesmen', 'POST', req, res, body)
})
router.get('/salesmen/:id', (req, res) => proxyToCRM(`/api/salesmen/${req.params.id}`, 'GET', req, res))
router.put('/salesmen/:id', (req, res) => {
    const body = JSON.stringify(req.body)
    proxyToCRM(`/api/salesmen/${req.params.id}`, 'PUT', req, res, body)
})
router.delete('/salesmen/:id', (req, res) => proxyToCRM(`/api/salesmen/${req.params.id}`, 'DELETE', req, res))

// Campaigns (internal CRUD)
router.get('/campaigns/check-slug', (req, res) => {
    const qs = new URLSearchParams(req.query as Record<string, string>).toString()
    proxyToCRM(`/api/campaigns/check-slug${qs ? '?' + qs : ''}`, 'GET', req, res)
})
router.get('/campaigns', (req, res) => proxyToCRM('/api/campaigns', 'GET', req, res))
router.post('/campaigns', (req, res) => {
    const body = JSON.stringify(req.body)
    proxyToCRM('/api/campaigns', 'POST', req, res, body)
})
router.get('/campaigns/:id', (req, res) => proxyToCRM(`/api/campaigns/${req.params.id}`, 'GET', req, res))
router.put('/campaigns/:id', (req, res) => {
    const body = JSON.stringify(req.body)
    proxyToCRM(`/api/campaigns/${req.params.id}`, 'PUT', req, res, body)
})
router.delete('/campaigns/:id', (req, res) => proxyToCRM(`/api/campaigns/${req.params.id}`, 'DELETE', req, res))

// Public campaign (no internal key — forward langsung)
function proxyPublic(path: string, method: string, req: express.Request, res: express.Response, body?: string) {
    const options = {
        hostname: '127.0.0.1',
        port: 8083,
        path,
        method,
        headers: {
            'Content-Type': 'application/json',
            ...(body ? { 'Content-Length': Buffer.byteLength(body) } : {})
        }
    }
    const proxyReq = http.request(options, (proxyRes) => {
        res.setHeader('Content-Type', 'application/json')
        res.setHeader('Access-Control-Allow-Origin', '*')
        res.status(proxyRes.statusCode || 200)
        proxyRes.pipe(res)
    })
    proxyReq.on('error', (e) => res.status(503).json({ error: 'CRM unavailable: ' + e.message }))
    if (body) proxyReq.write(body)
    proxyReq.end()
}

router.get('/campaigns/public/:slug', (req, res) => proxyPublic(`/api/public/campaigns/${req.params.slug}`, 'GET', req, res))
router.post('/campaigns/public/:slug/submit', (req, res) => {
    const body = JSON.stringify(req.body)
    proxyPublic(`/api/public/campaigns/${req.params.slug}/submit`, 'POST', req, res, body)
})
router.options('/campaigns/public/:slug/submit', (req, res) => {
    res.setHeader('Access-Control-Allow-Origin', '*')
    res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS')
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type')
    res.status(204).end()
})

export default router

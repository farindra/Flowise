import client from './client'

const BASE = '/crm-broadcast'

// Config & lookups
const getThrottleDefaults = () => client.get(`${BASE}/throttle-defaults`)
const getSenders = () => client.get(`${BASE}/senders`)
const getChatSources = () => client.get(`${BASE}/chat-sources`)
const getWilayah = () => client.get(`${BASE}/wilayah`)

// Opt-outs
const getOptOuts = () => client.get(`${BASE}/optouts`)
const addOptOut = (body) => client.post(`${BASE}/optouts`, body)
const removeOptOut = (phone) => client.delete(`${BASE}/optouts/${encodeURIComponent(phone)}`)

// Audience
const previewAudience = (body) => client.post(`${BASE}/preview-audience`, body)

// Multipart uploads must override the client's default JSON Content-Type,
// otherwise axios never generates a boundary and the server rejects the body.
const MULTIPART = { headers: { 'Content-Type': 'multipart/form-data' } }
const uploadAudienceFile = (formData) => client.post(`${BASE}/audience-file`, formData, MULTIPART)
const uploadMedia = (formData) => client.post(`${BASE}/media`, formData, MULTIPART)

// Campaigns
const getBroadcasts = (params) => client.get(`${BASE}/broadcasts`, { params })
const getBroadcast = (id) => client.get(`${BASE}/broadcasts/${id}`)
const createBroadcast = (body) => client.post(`${BASE}/broadcasts`, body)
const updateBroadcast = (id, body) => client.put(`${BASE}/broadcasts/${id}`, body)
const deleteBroadcast = (id) => client.delete(`${BASE}/broadcasts/${id}`)

// Audience build & results
const buildAudience = (id) => client.post(`${BASE}/broadcasts/${id}/audience`)
const getBuildStatus = (id, jobId) => client.get(`${BASE}/broadcasts/${id}/audience/${jobId}`)
const getRecipients = (id, params) => client.get(`${BASE}/broadcasts/${id}/recipients`, { params })

// Downloads go through axios as blobs: an <a download> navigation carries
// cookies but not the x-request-from header, so it would 401 on this route.
const downloadTemplate = () => client.get(`${BASE}/template`, { responseType: 'blob' })
const exportRecipients = (id) => client.get(`${BASE}/broadcasts/${id}/export`, { responseType: 'blob' })

// Controls
const testSend = (id, body) => client.post(`${BASE}/broadcasts/${id}/test-send`, body)
const startBroadcast = (id) => client.post(`${BASE}/broadcasts/${id}/start`)
const pauseBroadcast = (id) => client.post(`${BASE}/broadcasts/${id}/pause`)
const resumeBroadcast = (id) => client.post(`${BASE}/broadcasts/${id}/resume`)
const cancelBroadcast = (id) => client.post(`${BASE}/broadcasts/${id}/cancel`)
const retryBroadcast = (id, scope) => client.post(`${BASE}/broadcasts/${id}/retry`, { scope })

export default {
    getThrottleDefaults,
    getSenders,
    getChatSources,
    getWilayah,
    getOptOuts,
    addOptOut,
    removeOptOut,
    previewAudience,
    uploadAudienceFile,
    uploadMedia,
    getBroadcasts,
    getBroadcast,
    createBroadcast,
    updateBroadcast,
    deleteBroadcast,
    buildAudience,
    getBuildStatus,
    getRecipients,
    downloadTemplate,
    exportRecipients,
    testSend,
    startBroadcast,
    pauseBroadcast,
    resumeBroadcast,
    cancelBroadcast,
    retryBroadcast
}

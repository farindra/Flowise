import PropTypes from 'prop-types'
import { useEffect, useState, useCallback } from 'react'
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    Grid,
    IconButton,
    InputLabel,
    MenuItem,
    Paper,
    Select,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Typography
} from '@mui/material'
import { IconRefresh, IconPhone, IconUser, IconChevronRight } from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const API = '/api/v1/crm'

const STAGES = [
    { value: '', label: 'Semua Stage' },
    { value: 'new', label: '🆕 New' },
    { value: 'contacted', label: '📞 Contacted' },
    { value: 'qualified', label: '✅ Qualified' },
    { value: 'proposal', label: '📋 Proposal' },
    { value: 'negotiation', label: '🤝 Negotiation' },
    { value: 'won', label: '🎉 Won' },
    { value: 'lost', label: '❌ Lost' }
]

const STAGE_COLOR = {
    new: 'default',
    contacted: 'info',
    qualified: 'primary',
    proposal: 'warning',
    negotiation: 'secondary',
    won: 'success',
    lost: 'error'
}

const STAGE_EMOJI = { new: '🆕', contacted: '📞', qualified: '✅', proposal: '📋', negotiation: '🤝', won: '🎉', lost: '❌' }

const URGENCY_COLOR = { high: 'error', medium: 'warning', low: 'default' }

function StatsBar({ stats }) {
    if (!stats) return null
    const items = [
        { label: 'Total Lead', value: stats.total_leads ?? '—' },
        { label: 'New', value: stats.by_stage?.new ?? 0, color: '#64748b' },
        { label: 'Contacted', value: stats.by_stage?.contacted ?? 0, color: '#0ea5e9' },
        { label: 'Qualified', value: stats.by_stage?.qualified ?? 0, color: '#6366f1' },
        { label: 'Won', value: stats.by_stage?.won ?? 0, color: '#22c55e' },
        { label: 'Lost', value: stats.by_stage?.lost ?? 0, color: '#ef4444' }
    ]
    return (
        <Grid container spacing={1.5} sx={{ mb: 2 }}>
            {items.map((it) => (
                <Grid item key={it.label}>
                    <Paper variant='outlined' sx={{ px: 2, py: 1, minWidth: 90, textAlign: 'center' }}>
                        <Typography variant='h5' sx={{ color: it.color, fontWeight: 700 }}>
                            {it.value}
                        </Typography>
                        <Typography variant='caption' color='text.secondary'>
                            {it.label}
                        </Typography>
                    </Paper>
                </Grid>
            ))}
        </Grid>
    )
}

StatsBar.propTypes = { stats: PropTypes.object }

function EditLeadDialog({ lead, onClose, onSaved }) {
    const [stage, setStage] = useState(lead?.stage || 'new')
    const [notes, setNotes] = useState(lead?.notes || '')
    const [assignedTo, setAssignedTo] = useState(lead?.assigned_to || '')
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState(null)

    const save = async () => {
        setSaving(true)
        setError(null)
        try {
            const res = await fetch(`${API}/leads/${lead.id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ stage, notes, assigned_to: assignedTo })
            })
            if (!res.ok) throw new Error('Gagal simpan')
            onSaved()
            onClose()
        } catch (e) {
            setError(e.message)
        } finally {
            setSaving(false)
        }
    }

    if (!lead) return null
    return (
        <Dialog open onClose={onClose} maxWidth='sm' fullWidth>
            <DialogTitle>Edit Lead — {lead.name || lead.phone}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{ mt: 1 }}>
                    {error && <Alert severity='error'>{error}</Alert>}
                    <FormControl fullWidth size='small'>
                        <InputLabel>Stage</InputLabel>
                        <Select value={stage} label='Stage' onChange={(e) => setStage(e.target.value)}>
                            {STAGES.filter((s) => s.value).map((s) => (
                                <MenuItem key={s.value} value={s.value}>
                                    {s.label}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    <TextField
                        size='small'
                        label='Assigned To'
                        value={assignedTo}
                        onChange={(e) => setAssignedTo(e.target.value)}
                        placeholder='Nama salesman'
                    />
                    <TextField size='small' label='Catatan' multiline rows={3} value={notes} onChange={(e) => setNotes(e.target.value)} />
                    <Divider />
                    <Stack direction='row' spacing={1} justifyContent='flex-end'>
                        <Chip label='Batal' onClick={onClose} variant='outlined' clickable />
                        <Chip
                            label={saving ? 'Menyimpan...' : 'Simpan'}
                            color='primary'
                            onClick={save}
                            disabled={saving}
                            clickable
                            icon={saving ? <CircularProgress size={12} color='inherit' /> : undefined}
                        />
                    </Stack>
                </Stack>
            </DialogContent>
        </Dialog>
    )
}

EditLeadDialog.propTypes = {
    lead: PropTypes.object,
    onClose: PropTypes.func.isRequired,
    onSaved: PropTypes.func.isRequired
}

export default function CRMLeads() {
    const [leads, setLeads] = useState([])
    const [stats, setStats] = useState(null)
    const [stageFilter, setStageFilter] = useState('')
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState(null)
    const [editLead, setEditLead] = useState(null)

    const fetchLeads = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const qs = stageFilter ? `?stage=${stageFilter}` : ''
            const [leadsRes, statsRes] = await Promise.all([fetch(`${API}/leads${qs}`), fetch(`${API}/stats?period=month`)])
            const leadsData = await leadsRes.json()
            const statsData = await statsRes.json()
            setLeads(Array.isArray(leadsData) ? leadsData : [])
            setStats(statsData)
        } catch (e) {
            setError(e.message)
        } finally {
            setLoading(false)
        }
    }, [stageFilter])

    useEffect(() => {
        fetchLeads()
    }, [fetchLeads])

    const fmtDate = (s) => (s ? new Date(s).toLocaleDateString('id-ID', { day: '2-digit', month: 'short', year: '2-digit' }) : '—')

    return (
        <MainCard
            title='CRM — Pipeline Leads'
            secondary={
                <IconButton size='small' onClick={fetchLeads} disabled={loading}>
                    {loading ? <CircularProgress size={16} /> : <IconRefresh size={18} />}
                </IconButton>
            }
        >
            <StatsBar stats={stats} />

            <Stack direction='row' spacing={1.5} sx={{ mb: 2 }} alignItems='center'>
                <FormControl size='small' sx={{ minWidth: 180 }}>
                    <InputLabel>Filter Stage</InputLabel>
                    <Select value={stageFilter} label='Filter Stage' onChange={(e) => setStageFilter(e.target.value)}>
                        {STAGES.map((s) => (
                            <MenuItem key={s.value} value={s.value}>
                                {s.label}
                            </MenuItem>
                        ))}
                    </Select>
                </FormControl>
                <Typography variant='caption' color='text.secondary'>
                    {leads.length} lead{leads.length !== 1 ? 's' : ''}
                </Typography>
            </Stack>

            {error && (
                <Alert severity='error' sx={{ mb: 2 }}>
                    {error}
                </Alert>
            )}

            <TableContainer component={Paper} variant='outlined'>
                <Table size='small'>
                    <TableHead>
                        <TableRow>
                            <TableCell>Nama</TableCell>
                            <TableCell>Telepon</TableCell>
                            <TableCell>Stage</TableCell>
                            <TableCell>Urgensi</TableCell>
                            <TableCell>Source</TableCell>
                            <TableCell>Assigned</TableCell>
                            <TableCell>Masuk</TableCell>
                            <TableCell sx={{ width: 36 }} />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {leads.length === 0 && !loading && (
                            <TableRow>
                                <TableCell colSpan={8} align='center' sx={{ py: 4, color: 'text.disabled' }}>
                                    Belum ada leads
                                </TableCell>
                            </TableRow>
                        )}
                        {leads.map((lead) => (
                            <TableRow key={lead.id} hover sx={{ cursor: 'pointer' }} onClick={() => setEditLead(lead)}>
                                <TableCell>
                                    <Stack direction='row' spacing={0.5} alignItems='center'>
                                        <IconUser size={14} />
                                        <Typography variant='body2'>{lead.name || <em style={{ color: '#888' }}>—</em>}</Typography>
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Stack direction='row' spacing={0.5} alignItems='center'>
                                        <IconPhone size={14} />
                                        <Typography variant='body2' sx={{ fontFamily: 'monospace' }}>
                                            {lead.phone}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Chip
                                        size='small'
                                        label={`${STAGE_EMOJI[lead.stage] || ''} ${lead.stage}`}
                                        color={STAGE_COLOR[lead.stage] || 'default'}
                                        sx={{ fontSize: 11 }}
                                    />
                                </TableCell>
                                <TableCell>
                                    <Chip
                                        size='small'
                                        label={lead.urgency}
                                        color={URGENCY_COLOR[lead.urgency] || 'default'}
                                        sx={{ fontSize: 11 }}
                                    />
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='text.secondary'>
                                        {lead.source}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='text.secondary'>
                                        {lead.assigned_to || '—'}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <Typography variant='caption' color='text.secondary'>
                                        {fmtDate(lead.created_at)}
                                    </Typography>
                                </TableCell>
                                <TableCell>
                                    <IconChevronRight size={16} color='#888' />
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </TableContainer>

            {editLead && <EditLeadDialog lead={editLead} onClose={() => setEditLead(null)} onSaved={fetchLeads} />}

            <Box sx={{ mt: 1 }}>
                <Typography variant='caption' color='text.disabled'>
                    Klik baris untuk edit stage, catatan, atau assigned salesman.
                </Typography>
            </Box>
        </MainCard>
    )
}

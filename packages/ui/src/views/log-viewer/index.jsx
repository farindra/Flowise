import PropTypes from 'prop-types'
import { useState } from 'react'
import {
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    IconButton,
    InputAdornment,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Typography,
    Alert,
    Paper
} from '@mui/material'
import {
    IconSearch,
    IconChevronDown,
    IconChevronUp,
    IconRefresh,
    IconAlertTriangle,
    IconInfoCircle,
    IconAlertCircle
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'

const SEARCH_URL = '/api/v1/log-search/search'

const levelColor = {
    ERROR: 'error',
    WARN: 'warning',
    INFO: 'info'
}

const levelIcon = {
    ERROR: <IconAlertCircle size={14} />,
    WARN: <IconAlertTriangle size={14} />,
    INFO: <IconInfoCircle size={14} />
}

function ExpandableRow({ row }) {
    const [open, setOpen] = useState(false)
    return (
        <>
            <TableRow hover sx={{ cursor: 'pointer' }} onClick={() => setOpen(!open)}>
                <TableCell sx={{ width: 36, py: 1 }}>
                    <IconButton size='small'>{open ? <IconChevronUp size={16} /> : <IconChevronDown size={16} />}</IconButton>
                </TableCell>
                <TableCell sx={{ py: 1 }}>
                    <Chip
                        size='small'
                        label={row.level || 'INFO'}
                        color={levelColor[row.level] || 'default'}
                        icon={levelIcon[row.level]}
                        sx={{ fontWeight: 700, fontSize: 11 }}
                    />
                </TableCell>
                <TableCell sx={{ py: 1 }}>
                    {row.code ? (
                        <Chip
                            size='small'
                            label={row.code}
                            sx={{ fontFamily: 'monospace', fontWeight: 700, bgcolor: '#fff8e1', color: '#f57f17', fontSize: 12 }}
                        />
                    ) : (
                        <Typography variant='caption' color='text.disabled'>
                            —
                        </Typography>
                    )}
                </TableCell>
                <TableCell sx={{ py: 1 }}>
                    <Chip size='small' label={row.source || '—'} variant='outlined' sx={{ fontSize: 11 }} />
                </TableCell>
                <TableCell sx={{ py: 1, maxWidth: 420 }}>
                    <Typography variant='body2' noWrap sx={{ color: 'text.secondary', fontFamily: 'monospace', fontSize: 12 }}>
                        {(row.message || '').slice(0, 140)}
                    </Typography>
                </TableCell>
                <TableCell sx={{ py: 1, whiteSpace: 'nowrap' }}>
                    <Typography variant='caption' color='text.disabled'>
                        {(row.time || '').replace('T', ' ').replace('Z', '')}
                    </Typography>
                </TableCell>
            </TableRow>
            <TableRow>
                <TableCell colSpan={6} sx={{ py: 0, border: 0 }}>
                    <Collapse in={open} unmountOnExit>
                        <Box sx={{ m: 1, p: 2, bgcolor: 'grey.950', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                            <Typography
                                component='pre'
                                sx={{
                                    fontFamily: 'monospace',
                                    fontSize: 12,
                                    whiteSpace: 'pre-wrap',
                                    wordBreak: 'break-all',
                                    m: 0,
                                    color: 'text.secondary'
                                }}
                            >
                                {row.message}
                            </Typography>
                        </Box>
                    </Collapse>
                </TableCell>
            </TableRow>
        </>
    )
}

ExpandableRow.propTypes = {
    row: PropTypes.shape({
        level: PropTypes.string,
        code: PropTypes.string,
        source: PropTypes.string,
        message: PropTypes.string,
        time: PropTypes.string
    })
}

export default function LogViewer() {
    const [query, setQuery] = useState('')
    const [date, setDate] = useState(new Date().toISOString().slice(0, 10))
    const [results, setResults] = useState(null)
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState(null)

    const search = async () => {
        const q = query.trim().toUpperCase()
        if (!q) return
        setLoading(true)
        setError(null)
        try {
            const res = await fetch(`${SEARCH_URL}?q=${encodeURIComponent(q)}&date=${date}`)
            const data = await res.json()
            if (data.error) throw new Error(data.error)
            setResults(data)
        } catch (e) {
            setError(e.message)
            setResults(null)
        } finally {
            setLoading(false)
        }
    }

    const handleKey = (e) => {
        if (e.key === 'Enter') search()
    }

    const handleQueryChange = (e) => {
        setQuery(e.target.value.toUpperCase())
    }

    return (
        <MainCard title='Log Viewer'>
            <Stack spacing={2}>
                <Stack direction='row' spacing={1.5} alignItems='center' flexWrap='wrap'>
                    <TextField
                        size='small'
                        placeholder='Cari error code (A3K9M) atau keyword...'
                        value={query}
                        onChange={handleQueryChange}
                        onKeyDown={handleKey}
                        sx={{ minWidth: 280, fontFamily: 'monospace', flex: 1 }}
                        InputProps={{
                            startAdornment: (
                                <InputAdornment position='start'>
                                    <IconSearch size={16} />
                                </InputAdornment>
                            ),
                            sx: { fontFamily: 'monospace', letterSpacing: '0.05em' }
                        }}
                    />
                    <TextField size='small' type='date' value={date} onChange={(e) => setDate(e.target.value)} sx={{ width: 160 }} />
                    <Button
                        variant='contained'
                        onClick={search}
                        disabled={loading || !query.trim()}
                        startIcon={loading ? <CircularProgress size={14} color='inherit' /> : <IconSearch size={16} />}
                    >
                        {loading ? 'Mencari...' : 'Cari'}
                    </Button>
                    {results && (
                        <IconButton size='small' onClick={search} title='Refresh'>
                            <IconRefresh size={16} />
                        </IconButton>
                    )}
                </Stack>

                <Typography variant='caption' color='text.disabled'>
                    Cari by error code 5 huruf dari pesan Telegram, atau keyword lain. Source: salesman-service + Flowise server logs.
                </Typography>

                {error && <Alert severity='error'>{error}</Alert>}

                {results && results.total === 0 && (
                    <Alert severity='info'>
                        Tidak ada hasil untuk &quot;{results.query}&quot; pada {results.date}
                    </Alert>
                )}

                {results && results.total > 0 && (
                    <Box>
                        <Typography variant='caption' color='text.secondary' sx={{ mb: 1, display: 'block' }}>
                            {results.total} hasil untuk &quot;{results.query}&quot; — {results.date}
                        </Typography>
                        <TableContainer component={Paper} variant='outlined'>
                            <Table size='small'>
                                <TableHead>
                                    <TableRow>
                                        <TableCell sx={{ width: 36 }} />
                                        <TableCell sx={{ width: 80 }}>Level</TableCell>
                                        <TableCell sx={{ width: 100 }}>Kode</TableCell>
                                        <TableCell sx={{ width: 140 }}>Source</TableCell>
                                        <TableCell>Pesan</TableCell>
                                        <TableCell sx={{ width: 150 }}>Waktu</TableCell>
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {results.results.map((row, i) => (
                                        <ExpandableRow key={i} row={row} />
                                    ))}
                                </TableBody>
                            </Table>
                        </TableContainer>
                    </Box>
                )}
            </Stack>
        </MainCard>
    )
}

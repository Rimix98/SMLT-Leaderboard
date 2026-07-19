import { fetchWithAbort, parseJsonResponse, BACKEND_URL } from './utils'

export interface SMPStatus {
  online: boolean
  playersMax: number
  playersOnline: number
  version: string
  serverIp: string
  fetchedAt: string
}

export async function fetchSMPStatus(): Promise<SMPStatus> {
  const res = await fetchWithAbort(`${BACKEND_URL}/smp/status`, {
    method: 'GET',
  }, 'smp-status')
  if (!res.ok) throw new Error('Failed to fetch SMP status')
  const data = await parseJsonResponse(res)
  return data as unknown as SMPStatus
}

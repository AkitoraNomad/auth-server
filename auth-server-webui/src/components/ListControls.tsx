import { useState } from 'react'
import type { ListQuery } from '../types'

interface Props {
  page: number
  pageSize: number
  total: number
  query: string
  status: ListQuery['status']
  loading: boolean
  setPage: (page: number) => void
  setPageSize: (size: number) => void
  setQuery: (query: string) => void
  setStatus: (status: ListQuery['status']) => void
}
export function ListControls(props: Props) {
  const [draft, setDraft] = useState(props.query)
  const pages = Math.max(1, Math.ceil(props.total / props.pageSize))
  return <div className="flex flex-wrap items-end justify-between gap-3 rounded-xl border border-base-300 p-3">
    <form className="flex flex-wrap items-end gap-2" onSubmit={(event) => { event.preventDefault(); props.setQuery(draft) }}>
      <label className="flex flex-col gap-1 text-sm">Search
        <input type="search" className="input input-sm w-48" maxLength={200} value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="Name, email or ID" />
      </label>
      <button className="btn btn-sm" disabled={props.loading}>Search</button>
      <label className="flex flex-col gap-1 text-sm">Status
        <select aria-label="Status" className="select select-sm" value={props.status} disabled={props.loading} onChange={(event) => props.setStatus(event.target.value as ListQuery['status'])}>
          <option value="all">All statuses</option><option value="active">Active</option><option value="disabled">Disabled</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">Page size
        <select aria-label="Page size" className="select select-sm" value={props.pageSize} disabled={props.loading} onChange={(event) => props.setPageSize(Number(event.target.value))}>
          {[10, 20, 50, 100].map((value) => <option key={value} value={value}>{value}</option>)}
        </select>
      </label>
    </form>
    <div className="flex items-center gap-2 text-sm">
      <span aria-live="polite">{props.total} results · Page {props.page} / {pages}</span>
      <button className="btn btn-sm" disabled={props.loading || props.page <= 1} onClick={() => props.setPage(props.page - 1)}>Previous</button>
      <button className="btn btn-sm" disabled={props.loading || props.page >= pages} onClick={() => props.setPage(props.page + 1)}>Next</button>
    </div>
  </div>
}

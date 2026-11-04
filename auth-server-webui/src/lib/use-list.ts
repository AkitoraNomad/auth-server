import { useEffect, useReducer, useState } from 'react'
import { errorMessage } from './api'
import type { ListQuery, Page } from '../types'

export function useList<T>(load: (query: ListQuery) => Promise<Page<T>>) {
  const [data, setData] = useState<T[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [page, updatePage] = useState(1)
  const [pageSize, updatePageSize] = useState(20)
  const [query, updateQuery] = useState('')
  const [status, updateStatus] = useState<ListQuery['status']>('all')
  const [total, setTotal] = useState(0)
  const [version, invalidate] = useReducer((value: number) => value + 1, 0)

  useEffect(() => {
    let active = true
    load({ page, page_size: pageSize, q: query, status }).then((result) => {
      if (active) {
        setData(result.items)
        setTotal(result.total)
        updatePage(result.page)
      }
    }).catch((err: unknown) => {
      if (active) setError(errorMessage(err))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [load, version, page, pageSize, query, status])

  const reload = () => { setLoading(true); setError(null); invalidate() }
  const setPage = (value: number) => { setLoading(true); setError(null); updatePage(value) }
  const setPageSize = (value: number) => { updatePageSize(value); updatePage(1); reload() }
  const setQuery = (value: string) => { updateQuery(value.trim()); updatePage(1); reload() }
  const setStatus = (value: ListQuery['status']) => { updateStatus(value); updatePage(1); reload() }
  return { data, loading, error, setError, reload, page, pageSize, query, status, total, setPage, setPageSize, setQuery, setStatus }
}

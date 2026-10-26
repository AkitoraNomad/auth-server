export function ErrorAlert({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="alert alert-error mb-4">
      <span>{message}</span>
      {onRetry && (
        <button className="btn btn-sm" onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  )
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="text-center py-10 text-base-content/60">
      <p className="font-semibold">{title}</p>
      <p className="text-sm mt-1">{description}</p>
    </div>
  )
}

export function LoadingBlock({ label = 'Loading...' }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 py-10">
      <span className="loading loading-spinner loading-md" />
      <span className="text-base-content/60">{label}</span>
    </div>
  )
}

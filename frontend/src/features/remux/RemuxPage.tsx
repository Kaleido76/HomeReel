import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Loader2, RefreshCw } from 'lucide-react'
import { fetchRemuxStatus, requestRemux } from '../../api/remux'
import { jobsKey } from '../jobs/useJobs'

// RemuxPage is the segmented-MP4 remux manager: hls.js-downloaded files (many
// mdat boxes) make desktop Chrome download the whole file before playing. A
// remux re-wraps such a file into a standard faststart MP4 (stream copy, no
// re-encode) after which direct playback is fast and fully seekable. Remuxing
// is user-driven — nothing runs automatically at scan time.
export function RemuxPage() {
  const queryClient = useQueryClient()
  const [requested, setRequested] = useState<Set<string>>(new Set())

  const status = useQuery({
    queryKey: ['remux', 'status'],
    queryFn: fetchRemuxStatus,
    refetchInterval: 5000,
  })

  const items = status.data?.items ?? []

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['remux', 'status'] })

  const remuxOne = useMutation({
    mutationFn: requestRemux,
    onSuccess: (_, id) => {
      setRequested((prev) => new Set(prev).add(id))
      void invalidate()
      // 重封是后台任务，立即刷新任务指示器让顶部图标马上旋转。
      void queryClient.invalidateQueries({ queryKey: jobsKey })
    },
  })

  const pendingRemux = items.some((it) => !it.remuxed)

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-neutral-900">重封管理</h1>
          <p className="mt-1 max-w-2xl text-sm text-neutral-500">
            部分从网页下载的视频（hls.js 拼接）在电脑浏览器里会整文件下载后才能播放。重封可将其转换为标准
            faststart MP4（仅重封装、不重编码），之后秒开且可拖动。
          </p>
        </div>
        <button
          onClick={() => {
            const ids = items.filter((it) => !it.remuxed).map((it) => it.video_id)
            ids.forEach((id) => remuxOne.mutate(id))
          }}
          disabled={!pendingRemux || remuxOne.isPending}
          className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-40"
        >
          {remuxOne.isPending ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
          重封全部未重封
        </button>
      </div>

      {status.isLoading ? (
        <div className="flex h-40 items-center justify-center text-neutral-400">
          <Loader2 className="size-6 animate-spin" />
        </div>
      ) : status.isError ? (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-8 text-center text-sm text-red-600">
          {status.error.message}
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-lg border border-neutral-200 bg-white px-4 py-16 text-center text-neutral-400">
          暂无分段视频。检测到 hls.js 拼接的 MP4 时会出现在这里。
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-neutral-200 bg-white">
          <ul className="divide-y divide-neutral-100">
            {items.map((it) => {
              const done = it.remuxed
              const submitted = requested.has(it.video_id) && !done
              return (
                <li key={it.video_id} className="flex items-center gap-3 px-3 py-2.5">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-neutral-800" title={it.title}>
                      {it.title}
                    </p>
                    <p className="truncate text-xs text-neutral-400" title={it.relative_path}>
                      {it.relative_path}
                    </p>
                  </div>
                  <span
                    className={`shrink-0 rounded px-2 py-0.5 text-xs ${
                      done ? 'bg-emerald-50 text-emerald-600' : 'bg-neutral-100 text-neutral-500'
                    }`}
                  >
                    {done ? '已重封' : submitted ? '已提交' : '未重封'}
                  </span>
                  <button
                    onClick={() => remuxOne.mutate(it.video_id)}
                    disabled={done || submitted || remuxOne.isPending}
                    className="flex shrink-0 items-center gap-1 rounded-md border border-neutral-200 bg-white px-2.5 py-1 text-sm text-neutral-600 hover:bg-neutral-50 disabled:opacity-40"
                  >
                    {done ? <CheckCircle2 className="size-4 text-emerald-600" /> : <RefreshCw className="size-3.5" />}
                    {done ? '完成' : submitted ? '排队中' : '重封'}
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}

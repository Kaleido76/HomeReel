import { fetchMe } from '../../api/auth'
import { useQuery } from '@tanstack/react-query'

export function HomePage() {
  const me = useQuery({ queryKey: ['me'], queryFn: fetchMe })

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold text-neutral-900">
        欢迎，{me.data?.user ?? '…'}
      </h1>
      <p className="text-neutral-600">
        Phase 0 骨架已就绪：认证、路由与布局壳。后续阶段将在此填充首页行式浏览。
      </p>
    </div>
  )
}

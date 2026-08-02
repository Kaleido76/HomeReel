import { useLocation, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FolderOpen } from 'lucide-react'
import { fetchStorages } from '../../api/storages'
import { Breadcrumb } from './Breadcrumb'
import { FileList } from './FileList'
import { StorageSidebar } from './StorageSidebar'

interface ExplorerSearch {
  storageId: string
  path: string
}

export function ExplorerPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { storageId, path } = (location.search ?? {}) as ExplorerSearch

  const storages = useQuery({ queryKey: ['storages'], queryFn: fetchStorages })
  const selected = (storages.data?.storages ?? []).find((s) => s.id === storageId)

  function go(storageId: string, path: string) {
    navigate({ to: '/explorer', search: { storageId, path } })
  }

  return (
    <div className="flex h-full gap-4">
      <StorageSidebar
        storages={storages.data?.storages ?? []}
        isLoading={storages.isLoading}
        selectedId={storageId}
        onSelect={(id) => go(id, '')}
      />
      <div className="flex min-w-0 flex-1 flex-col rounded-xl border border-neutral-200 bg-white">
        {!storageId ? (
          <div className="flex h-full items-center justify-center text-neutral-400">
            <div className="text-center">
              <FolderOpen className="mx-auto mb-2 size-10" />
              <p>请先在左侧选择或添加存储卷</p>
            </div>
          </div>
        ) : (
          <>
            <div className="border-b border-neutral-100 px-4 py-2.5">
              <Breadcrumb storage={selected} path={path} onNavigate={(p) => go(storageId, p)} />
            </div>
            <div className="min-h-0 flex-1">
              <FileList storageId={storageId} path={path} onOpen={(p) => go(storageId, p)} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}

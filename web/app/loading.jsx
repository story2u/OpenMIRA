export default function Loading() {
  return (
    <main className="min-h-screen bg-[#f6f7f9] px-6 py-8">
      <div className="mx-auto max-w-7xl animate-pulse">
        <div className="h-7 w-44 rounded bg-[#d9dee8]" />
        <div className="mt-6 grid gap-4 md:grid-cols-3">
          <div className="h-28 rounded border border-[#d8dde8] bg-white" />
          <div className="h-28 rounded border border-[#d8dde8] bg-white" />
          <div className="h-28 rounded border border-[#d8dde8] bg-white" />
        </div>
      </div>
    </main>
  );
}

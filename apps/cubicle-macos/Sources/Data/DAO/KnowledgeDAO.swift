/// Database-backed knowledge graph access point.
protocol KnowledgeDAO {
    /// Connection policy and file layout shared across DAO implementations.
    var database: KnowledgeDatabase { get }
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/gerty-labs/gerty/internal/models"
)

// printJSON writes v as indented JSON to w.
func printJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// writeClusterReportTable writes a cluster report as a table to w.
func writeClusterReportTable(w io.Writer, report *models.ClusterReport) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "NAMESPACE\tOWNERS\tPODS\tCPU WASTE (m)\tMEM WASTE (MB)\n")
	for nsName, ns := range report.Namespaces {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%.0f\t%.0f\n",
			nsName,
			len(ns.Owners),
			len(ns.Pods),
			ns.TotalCPUWasteMillis,
			ns.TotalMemWasteBytes/1_000_000,
		)
	}
	tw.Flush()
}

// writeNamespaceReportTable writes a namespace report with per-owner breakdown.
func writeNamespaceReportTable(w io.Writer, report *models.NamespaceReport) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "OWNER\tKIND\tPODS\tCPU WASTE (m)\tMEM WASTE (MB)\n")
	for _, ow := range report.Owners {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%.0f\t%.0f\n",
			ow.Owner.Name,
			ow.Owner.Kind,
			ow.PodCount,
			ow.TotalCPUWasteMillis,
			ow.TotalMemWasteBytes/1_000_000,
		)
	}
	tw.Flush()
}

// writeRecommendationsTable writes recommendations sorted by savings descending.
func writeRecommendationsTable(w io.Writer, recs []models.Recommendation) {
	sort.Slice(recs, func(i, j int) bool {
		return recs[i].EstSavings > recs[j].EstSavings
	})

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "TARGET\tCONTAINER\tRESOURCE\tCURRENT\tRECOMMENDED\tSAVINGS\tRISK\tCONFIDENCE\n")
	for _, rec := range recs {
		target := fmt.Sprintf("%s/%s", rec.Target.Namespace, rec.Target.Name)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%.0f%%\n",
			target,
			rec.Container,
			rec.Resource,
			rec.CurrentRequest,
			rec.RecommendedReq,
			rec.EstSavings,
			rec.Risk,
			rec.Confidence*100,
		)
	}
	tw.Flush()
}

// writeWorkloadsTable writes a workload list table.
func writeWorkloadsTable(w io.Writer, workloads []models.OwnerWaste) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "NAMESPACE\tKIND\tNAME\tPODS\tCPU WASTE (m)\tMEM WASTE (MB)\n")
	for _, ow := range workloads {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%.0f\t%.0f\n",
			ow.Owner.Namespace,
			ow.Owner.Kind,
			ow.Owner.Name,
			ow.PodCount,
			ow.TotalCPUWasteMillis,
			ow.TotalMemWasteBytes/1_000_000,
		)
	}
	tw.Flush()
}

// writeWorkloadDetailTable writes a single workload detail with container breakdown.
func writeWorkloadDetailTable(w io.Writer, ow *models.OwnerWaste) {
	fmt.Fprintf(w, "Owner:     %s/%s (%s)\n", ow.Owner.Namespace, ow.Owner.Name, ow.Owner.Kind)
	fmt.Fprintf(w, "Pods:      %d\n", ow.PodCount)
	fmt.Fprintf(w, "CPU Waste: %.0fm\n", ow.TotalCPUWasteMillis)
	fmt.Fprintf(w, "Mem Waste: %.0f MB\n\n", ow.TotalMemWasteBytes/1_000_000)

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "CONTAINER\tCPU REQ (m)\tCPU P95 (m)\tCPU WASTE%%\tMEM REQ (MB)\tMEM P95 (MB)\tMEM WASTE%%\n")
	for _, c := range ow.Containers {
		fmt.Fprintf(tw, "%s\t%d\t%.0f\t%.0f%%\t%d\t%.0f\t%.0f%%\n",
			c.ContainerName,
			c.CPURequestMillis,
			c.CPUUsage.P95,
			c.CPUWastePercent,
			c.MemoryRequestBytes/1_000_000,
			c.MemoryUsage.P95/1_000_000,
			c.MemWastePercent,
		)
	}
	tw.Flush()
}

require 'active_record'
require_relative '../models/invoice'

module Billing
  class OrderService < BaseService
    include Logging
    extend ClassMethods

    attr_accessor :repository
    attr_reader :invoice_factory

    def initialize(repo)
      @repository = repo
      @invoice_factory = InvoiceFactory.new
    end

    def create(params)
      order = @repository.save(params)
      invoice = Invoice.new
      invoice
    end

    def cancel(order_id)
      order = @repository.find(order_id)
      order.cancel
    end

    private

    def validate(params)
      if params.nil?
        return false
      end
      true
    end
  end
end

module Billing
  class OrderService
    def create
      repository.save
      Invoice.new
    end

    def repository
      Repository.new
    end
  end
end
